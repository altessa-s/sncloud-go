// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

const (
	testClientID = "aabbccddeeff00112233445566778899"
	openFrame    = `0{"sid":"21739e0d-4372-4630-9e07-74de192b5e5e","upgrades":["websocket"],"pingInterval":5000,"pingTimeout":25000}`
)

func syntheticToken() string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":4102444800}`))
	return hdr + "." + claims + ".sig"
}

type blockingClock struct {
	now time.Time
}

func (c blockingClock) Now() time.Time { return c.now }
func (c blockingClock) Sleep(ctx context.Context, _ time.Duration) error {
	<-ctx.Done()
	return ctx.Err()
}

func eventFrame(name, jsonArg string) string {
	raw, _ := json.Marshal([]string{name, jsonArg})
	return "42" + string(raw)
}

func newTestFeed(
	endpoints domain.Endpoints,
	dialer port.SocketDialer,
	clk port.Clock,
) (*Feed, *porttest.Secrets) {
	secrets := &porttest.Secrets{}
	_ = secrets.Keep(syntheticToken(), domain.AccessToken)
	_ = secrets.Keep("synthetic@example.com", domain.Account)
	_ = secrets.Keep(testClientID, domain.ClientID)
	ch := channel.New(endpoints, &porttest.Transport{}, clk, domain.DefaultRetryPolicy())
	sess := session.New(ch, secrets, clk)
	return New(endpoints, sess, dialer, clk, domain.DefaultRetryPolicy()), secrets
}

func collectChanges(seq func(func(domain.Change, error) bool)) ([]domain.Change, error) {
	var out []domain.Change
	for ch, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, ch)
	}
	return out, nil
}

func TestOpensOfficialAndPrivateChannelsWithSignedQuery(t *testing.T) {
	clk := blockingClock{now: time.Unix(1780000000, 0)}
	sock := porttest.NewSocket(openFrame, "40", "40")
	dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sock}}}
	f, _ := newTestFeed(domain.Official(), dialer, clk)

	changes, err := collectChanges(f.Changes(context.Background()))
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != domain.ChangeConnected {
		t.Fatalf("changes = %+v", changes)
	}

	call := dialer.Calls()[0]
	u, err := url.Parse(call.URL)
	if err != nil || u.Scheme != "wss" || u.Host != "app-wss-file.supernote.com" || u.Path != "/socket.io/" {
		t.Fatalf("unexpected dial URL: %s (%v)", call.URL, err)
	}
	q := u.Query()
	wantType := domain.SocketType(domain.NewDeviceIdentity("", testClientID))
	wantSign := domain.FeedSignature(syntheticToken() + "_" + wantType + "_1780000000000")
	if q.Get("token") != syntheticToken() || q.Get("type") != wantType || q.Get("random") != "1780000000000" ||
		q.Get("sign") != wantSign || q.Get("EIO") != "3" || q.Get("transport") != "websocket" {
		t.Fatalf("unexpected query params: %+v", q)
	}
	if call.Header.Get("User-Agent") != domain.AppUserAgent {
		t.Fatalf("User-Agent = %q", call.Header.Get("User-Agent"))
	}
	if !slices.Contains(sock.Sent(), `42["ratta_ping","ping"]`) {
		t.Fatalf("expected initial ratta_ping, sent=%v", sock.Sent())
	}

	// Private plain HTTP uses ws:// on its own host and encodes '+' as %2B.
	privSock := porttest.NewSocket(openFrame, "40")
	privDialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: privSock}}}
	privFeed, _ := newTestFeed(domain.PrivateCloud("http://127.0.0.1:19072", false), privDialer, clk)
	if _, err := collectChanges(privFeed.Changes(context.Background())); err != nil {
		t.Fatalf("private Changes: %v", err)
	}
	if !strings.HasPrefix(privDialer.Calls()[0].URL, "ws://127.0.0.1:19072/socket.io/?") {
		t.Fatalf("private dial URL = %s", privDialer.Calls()[0].URL)
	}
	plusURL, err := handshakeURL("https://feed.example.test", "tok+plus", "MACOS1", "123")
	if err != nil || !strings.Contains(plusURL, "tok%2Bplus") {
		t.Fatalf("handshakeURL '+' encoding = (%s, %v)", plusURL, err)
	}
	if _, err := handshakeURL("://bad", "t", "k", "r"); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected invalid socket host error, got %v", err)
	}
}

func TestReadsAndAcknowledgesAnnouncementsAndHandlesRefusals(t *testing.T) {
	clk := blockingClock{now: time.Unix(1780000000, 0)}
	filesJSON := `{"code":"200","msg":null,"timestamp":1791049608000,"msgType":"FILE-SYN","data":[` +
		`{"messageType":"MOVEFILE","equipmentNo":"SN078C10034074-ANDROIDabc","fileType":"FILE","id":819922212654940160,` +
		`"originalName":"Meeting.note","md5":"33D1FAAF58F0D0EDC56F1BE35868FE80","size":239886,"directoryId":5,"goDirectoryId":6,` +
		`"originalPath":"NOTE/Note/","newPath":"NOTE/Note/Archive/","timestamp":1791049608000},` +
		`{"messageType":"ADDFOLDER","fileType":"FOLDE","id":7,"newName":"Archive","directoryId":5}]}`
	tasksJSON := `{"code":"200","msgType":"TASK-SYN","data":[{"messageType":"QUERY","taskListId":"1","taskId":"c0ffee","timestamp":1791049608000}]}`
	digestsJSON := `{"code":"200","msgType":"DIGEST-SYN","data":[{"messageType":"ADD_DIGEST","dataType":"2","id":894400223386796032,"parentUniqueIdentifier":"g-1"}]}`

	sock := porttest.NewSocket(
		openFrame, "40", `42["ratta_ping","Received"]`, "3",
		eventFrame("ServerMessage", filesJSON),
		eventFrame("to-do", tasksJSON),
		eventFrame("digest", digestsJSON),
		`42["ServerMessage","true"]`,
	)
	dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sock}}}
	f, _ := newTestFeed(domain.Official(), dialer, clk)

	changes, err := collectChanges(f.Changes(context.Background()))
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 4 ||
		changes[0].Type != domain.ChangeConnected ||
		len(changes[1].Files) != 2 || changes[1].Files[0].Kind != domain.ChangeFileMoved || changes[1].Files[1].IsFolder != true ||
		len(changes[2].Tasks) != 1 || changes[2].Tasks[0].Kind != domain.ChangeTasksChanged ||
		len(changes[3].Digests) != 1 || changes[3].Digests[0].Kind != domain.ChangeDigestAdded {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	sent := sock.Sent()
	for _, wantAck := range []string{`42["ServerMessage","Received"]`, `42["to-do","Received"]`, `42["digest","Received"]`} {
		if !slices.Contains(sent, wantAck) {
			t.Fatalf("missing ack %s in %v", wantAck, sent)
		}
	}

	// Token refusal invalidates the session token while preserving ClientID.
	tokSock := porttest.NewSocket(openFrame, "40", eventFrame("ServerMessage", `{"code":"403","msg":"token校验不通过，拒绝连接"}`))
	tokDialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: tokSock}}}
	tokFeed, secrets := newTestFeed(domain.Official(), tokDialer, clk)
	if _, err := collectChanges(tokFeed.Changes(context.Background())); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if storedTok, _ := secrets.Secret(domain.AccessToken); storedTok != "" {
		t.Fatalf("expected access token dropped, got %q", storedTok)
	}
	if cid, _ := secrets.Secret(domain.ClientID); cid != testClientID {
		t.Fatal("expected clientID preserved")
	}

	// Signature refusal surfaces as RejectedError.
	sigSock := porttest.NewSocket(openFrame, "40", eventFrame("ServerMessage", `{"code":"403","msg":"sign值校验不通过，拒绝连接"}`))
	sigDialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sigSock}}}
	sigFeed, _ := newTestFeed(domain.Official(), sigDialer, clk)
	var rej *domain.RejectedError
	if _, err := collectChanges(sigFeed.Changes(context.Background())); !errors.As(err, &rej) || rej.Code != "403" {
		t.Fatalf("expected RejectedError 403, got %v", err)
	}

	// Server close packet "41" ends stream cleanly and closes socket.
	closeSock := porttest.NewSocket(openFrame, "40", "41", eventFrame("to-do", `{"code":"200","data":[]}`))
	closeDialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: closeSock}}}
	closeFeed, _ := newTestFeed(domain.Official(), closeDialer, clk)
	gotClose, err := collectChanges(closeFeed.Changes(context.Background()))
	if err != nil || len(gotClose) != 1 || gotClose[0].Type != domain.ChangeConnected || !closeSock.Closed() {
		t.Fatalf("server close = (%+v, %v, closed=%v)", gotClose, err, closeSock.Closed())
	}
}

func TestEngineIOFrameParserAndReaderStop(t *testing.T) {
	if f, err := parseFrame(openFrame); err != nil || f.kind != frameOpen || f.interval != 5*time.Second || f.timeout != 25*time.Second {
		t.Fatalf("parseFrame open = (%+v, %v)", f, err)
	}
	if f, err := parseFrame("0{}"); err != nil || f.interval != 25*time.Second || f.timeout != 60*time.Second {
		t.Fatalf("parseFrame defaults = (%+v, %v)", f, err)
	}
	for _, tc := range []struct {
		raw  string
		want frameKind
	}{
		{"", frameIgnored},
		{"4", frameIgnored},
		{"49", frameIgnored},
		{"3", frameIgnored},
		{"6", frameIgnored},
		{"1", frameClosed},
		{"40", frameConnected},
		{"41", frameClosed},
		{`42["to-do","x"]`, frameEvent},
	} {
		f, err := parseFrame(tc.raw)
		if err != nil || f.kind != tc.want {
			t.Fatalf("parseFrame(%q) = (%+v, %v)", tc.raw, f, err)
		}
	}
	if _, err := parseFrame(`44"bad namespace"`); !errors.Is(err, domain.ErrRejected) {
		t.Fatalf("expected ErrRejected on 44, got %v", err)
	}
	for _, badEvent := range []string{"42nope", "42[]", "42[123]"} {
		if _, err := parseFrame(badEvent); !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("parseFrame(%q) = %v, want ErrMalformedResponse", badEvent, err)
		}
	}
	for _, ev := range []string{eventFiles, eventTasks, eventDigests} {
		if _, err := parseAnnouncement(ev, `{"code":"200","data":"not-array"}`); !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("expected malformed batch on %s, got %v", ev, err)
		}
		ch, err := parseAnnouncement(ev, `{"code":"200"}`)
		if err != nil || ch == nil {
			t.Fatalf("nil data batch on %s = (%+v, %v)", ev, ch, err)
		}
	}

	// Reader breaking out of range closes a quiet socket immediately.
	clk := blockingClock{now: time.Unix(1780000000, 0)}
	quiet := porttest.NewQuietSocket(openFrame, "40")
	dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: quiet}}}
	f, _ := newTestFeed(domain.Official(), dialer, clk)
	for ch, err := range f.Changes(context.Background()) {
		if err != nil {
			t.Fatalf("Changes: %v", err)
		}
		if ch.Type == domain.ChangeConnected {
			break
		}
	}
	if !quiet.Closed() {
		t.Fatal("expected socket closed when reader breaks out of loop")
	}
}

func TestHeartbeatPingFailureAndSilenceTimeout(t *testing.T) {
	// Virtual time advances only after the connected event and initial ping.
	synctest.Test(t, func(t *testing.T) {
		pingFailSock := &failOnEnginePingSocket{Socket: porttest.NewQuietSocket(openFrame, "40")}
		dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: pingFailSock}}}
		f, _ := newTestFeed(domain.Official(), dialer, port.SystemClock{})

		seen, err := collectChanges(f.Changes(context.Background()))
		var trErr *domain.TransportError
		if !errors.As(err, &trErr) || trErr.Reason != domain.ReasonConnectionLost {
			t.Fatalf("expected ReasonConnectionLost, got %v", err)
		}
		if len(seen) != 1 || seen[0].Type != domain.ChangeConnected || !pingFailSock.Closed() {
			t.Fatalf("seen=%+v closed=%v", seen, pingFailSock.Closed())
		}
	})

	// Silence timeout inside testing/synctest virtual time (zero real-time sleep).
	synctest.Test(t, func(t *testing.T) {
		quickOpen := `0{"sid":"s","upgrades":[],"pingInterval":1000,"pingTimeout":1000}`
		quiet := porttest.NewQuietSocket(quickOpen, "40")
		sd := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: quiet}}}
		sf, _ := newTestFeed(domain.Official(), sd, port.SystemClock{})

		_, serr := collectChanges(sf.Changes(context.Background()))
		var stErr *domain.TransportError
		if !errors.As(serr, &stErr) || stErr.Reason != domain.ReasonTimedOut {
			t.Fatalf("expected ReasonTimedOut on silent server, got %v", serr)
		}
		if !quiet.Closed() {
			t.Fatal("expected silent socket closed")
		}
	})
}

type failOnEnginePingSocket struct {
	*porttest.Socket
}

func (s *failOnEnginePingSocket) Write(ctx context.Context, data []byte) error {
	if string(data) == "2" {
		return errors.New("broken pipe")
	}
	return s.Socket.Write(ctx, data)
}

func TestWatchReconnectsAndStopsOnTerminalError(t *testing.T) {
	clk := &porttest.Clock{Time: time.Unix(1780000000, 0)}
	sock1 := porttest.NewSocket(openFrame, "40", "41")
	sock2 := porttest.NewSocket(openFrame, "40", eventFrame("to-do", `{"code":"200","data":[{"messageType":"QUERY","taskListId":"1"}]}`))
	dialer := &porttest.SocketDialer{
		Steps: []porttest.DialStep{
			{Socket: sock1},
			{Err: &domain.RateLimitedError{RetryAfter: 1, HasRetryAfter: true}},
			{Socket: sock2},
			{Err: domain.ErrSessionExpired},
		},
	}
	f, _ := newTestFeed(domain.Official(), dialer, clk)

	got, err := collectChanges(f.Watch(context.Background()))
	if !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatalf("expected terminal ErrSessionExpired, got %v", err)
	}
	if len(got) != 3 || got[0].Type != domain.ChangeConnected || got[1].Type != domain.ChangeConnected || got[2].Type != domain.ChangeTasks {
		t.Fatalf("unexpected Watch sequence: %+v", got)
	}
	if len(clk.Delays()) < 2 {
		t.Fatalf("expected reconnect backoff sleeps, got %v", clk.Delays())
	}

	// Early break in Watch stops cleanly without reconnecting.
	sockBreak := porttest.NewQuietSocket(openFrame, "40")
	dBreak := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sockBreak}}}
	fBreak, _ := newTestFeed(domain.Official(), dBreak, clk)
	for ch, werr := range fBreak.Watch(context.Background()) {
		if werr != nil {
			t.Fatal(werr)
		}
		if ch.Type == domain.ChangeConnected {
			break
		}
	}
	if !sockBreak.Closed() {
		t.Fatal("expected Watch socket closed on break")
	}
}
