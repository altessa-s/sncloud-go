// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	sncloud "github.com/altessa-s/sncloud-go"
)

// memoryStore is a concurrency-safe, process-local SecretStore. Its zero value is ready.
// A persistent application can replace it with an OS credential store.
type memoryStore struct {
	mu     sync.Mutex
	values map[sncloud.Secret]string
}

func (s *memoryStore) Secret(key sncloud.Secret) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *memoryStore) Keep(value string, key sncloud.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[sncloud.Secret]string)
	}
	s.values[key] = value
	return nil
}

func (s *memoryStore) Discard(key sncloud.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

// Tests supply emulator options; a real program leaves endpoints at their default.
// These nil options make the copied examples use the public service defaults.
var exampleOptions = func() []sncloud.Option { return nil }
var verificationOptions = func() []sncloud.Option { return nil }
var feedOptions = func() []sncloud.Option { return nil }

func ExampleNew() {
	store := &memoryStore{}
	client, err := sncloud.New(
		store,
		sncloud.WithLogger(slog.Default()),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	loggedIn, err := client.IsLoggedIn()
	fmt.Println("logged in:", loggedIn, "err:", err)
	// Output:
	// logged in: false err: <nil>
}

func ExampleClient_LogIn() {
	client, err := sncloud.New(
		&memoryStore{},
		verificationOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	outcome, err := client.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	if outcome == sncloud.VerificationRequired {
		if err := client.SendVerificationCode(ctx); err != nil {
			fmt.Println(err)
			return
		}
		if err := client.Verify(ctx, "123456"); err != nil {
			fmt.Println(err)
			return
		}
	}

	loggedIn, err := client.IsLoggedIn()
	if err != nil {
		fmt.Println(err)
		return
	}
	account, err := client.Account()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(outcome, loggedIn, account)
	// Output:
	// verificationRequired true reader@example.invalid
}

func ExampleFiles() {
	client, err := sncloud.New(
		&memoryStore{},
		exampleOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	if _, err := client.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
		fmt.Println(err)
		return
	}

	folder, err := client.Files().CreateFolder(ctx, "Notes", sncloud.RootFileID)
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := client.Files().UploadBytes(ctx, []byte("synthetic note body"), "Journal.note", folder.ID); err != nil {
		fmt.Println(err)
		return
	}

	entries, err := client.Files().List(ctx, folder.ID, sncloud.SortName, true)
	if err != nil {
		fmt.Println(err)
		return
	}

	tmpPath, err := client.Files().Download(ctx, entries[0].ID, entries[0].Size, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.Remove(tmpPath)

	body, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s/%s: %s\n", folder.Name, entries[0].Name, body)
	// Output:
	// Notes/Journal.note: synthetic note body
}

func ExampleDevice() {
	client, err := sncloud.New(
		&memoryStore{},
		exampleOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	if _, err := client.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
		fmt.Println(err)
		return
	}

	uploaded, err := client.Device().UploadBytes(ctx, []byte("0123456789"), "EXPORT/Sample.txt")
	if err != nil {
		fmt.Println(err)
		return
	}

	tmpPath, err := client.Device().Download(ctx, uploaded.ID, &sncloud.ByteRange{First: 2, Last: 6}, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.Remove(tmpPath)

	span, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s slice=%s\n", uploaded.Name, span)
	// Output:
	// Sample.txt slice=23456
}

func ExampleTasks() {
	client, err := sncloud.New(
		&memoryStore{},
		exampleOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	if _, err := client.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
		fmt.Println(err)
		return
	}

	list, err := client.Tasks().CreateList(ctx, "Reading", "")
	if err != nil {
		fmt.Println(err)
		return
	}
	created, err := client.Tasks().Create(ctx, sncloud.Task{
		ListID: &list.ID,
		Title:  "Review chapter notes",
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	items, err := client.Tasks().InList(ctx, list.ID)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s: %d task (%s, completed=%v)\n", list.Title, len(items), created.Title, items[0].IsCompleted)
	// Output:
	// Reading: 1 task (Review chapter notes, completed=false)
}

func ExampleDigests() {
	client, err := sncloud.New(
		&memoryStore{},
		exampleOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	if _, err := client.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
		fmt.Println(err)
		return
	}

	uid := sncloud.NewDigestIdentifier()
	if _, err := client.Digests().CreateGroup(ctx, "Articles", nil, uid); err != nil {
		fmt.Println(err)
		return
	}

	passage := "Key excerpt from a synthetic paper."
	tags := []string{"reference"}
	if _, err := client.Digests().Create(ctx, sncloud.DigestDraft{
		GroupIdentifier: &uid,
		Content:         &passage,
		Tags:            &tags,
	}); err != nil {
		fmt.Println(err)
		return
	}

	digests, err := client.Digests().List(ctx, uid)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("digests=%d tag=%s content=%q\n", len(digests), digests[0].Tags[0], *digests[0].Content)
	// Output:
	// digests=1 tag=reference content="Key excerpt from a synthetic paper."
}

func ExampleFeed_Watch() {
	watcher, err := sncloud.New(
		&memoryStore{},
		feedOptions()...,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer watcher.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := watcher.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
		fmt.Println(err)
		return
	}
	for change, err := range watcher.Feed().Watch(ctx) {
		if err != nil {
			fmt.Println(err)
			return
		}
		switch change.Type {
		case sncloud.ChangeConnected:
			// Reconcile state over REST on every fresh connection.
			lists, err := watcher.Tasks().Lists(ctx)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("connected, initial lists:", len(lists))

		case sncloud.ChangeTasks:
			// An announcement is only a hint; read the authoritative REST view.
			lists, err := watcher.Tasks().Lists(ctx)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("after announcement, lists:", len(lists))
			return
		case sncloud.ChangeFiles, sncloud.ChangeDigests:
		}
	}
	// Output:
	// connected, initial lists: 1
	// after announcement, lists: 2
}
