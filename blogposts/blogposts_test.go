package blogposts_test

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	blogposts "github.com/wagnercode88/blogposts"
)

type StubFailingFS struct {
}

func (s StubFailingFS) Open(name string) (fs.File, error) {
	return nil, errors.New("oh no, i always fail")
}

func TestNewBlogPosts(t *testing.T) {
	const (
		firstBody = `Title: Post 1
Description: Description 1
Tags: tdd, go
---
Hello world!

The body of posts starts after the`
		secondBody = `Title: Post 2
Description: Description 2
Tags: rust, borrow-checker
---
Hello world!

The body of posts starts after the`
	)

	fs := fstest.MapFS{
		"hello world.md":  {Data: []byte(firstBody)},
		"hello-world2.md": {Data: []byte(secondBody)},
	}

	posts, err := blogposts.NewPostsFromFS(fs)
	if err != nil {
		t.Fatal(err)
	}

	assertPost(t, posts[0], blogposts.Post{
		Title:       "Post 1",
		Description: "Description 1",
		Tags:        []string{"tdd", "go"},
		Body: `Hello world!

The body of posts starts after the`,
	})
}

func assertPost(t *testing.T, got blogposts.Post, want blogposts.Post) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestNewBlogPosts2(t *testing.T) {
	// fs := fstest.MapFS{
	// 	"hello world.md":  {Data: []byte("hi")},
	// 	"hello-world2.md": {Data: []byte("hola")},
	// }

	// later
	_, err := blogposts.NewPostsFromFS(StubFailingFS{})

	if err == nil {
		t.Fatal(err)
	}
}
