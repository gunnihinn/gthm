package atom

import (
	"fmt"
	"html/template"
	"time"

	"gthm/pkg/db"
)

type Feed struct {
	Header  template.HTML
	URL     string
	ID      string
	Entries []Entry
	updated time.Time
}

func (f Feed) Updated() string {
	return f.updated.Format(time.RFC3339)
}

func FromPosts(posts []db.Post, address string) Feed {
	feed := Feed{
		ID:     address,
		URL:    fmt.Sprintf("%s/feed", address),
		Header: template.HTML(`<?xml version="1.0" encoding="utf-8"?>`),
	}
	if len(posts) > 0 {
		feed.updated = fromUnixEpoch(posts[0].Created)
	} else {
		feed.updated = time.Now()
	}
	for _, post := range posts {
		feed.Entries = append(feed.Entries, fromPost(post, address))
	}

	return feed
}

type Entry struct {
	Title   string
	ID      string
	URL     string
	updated time.Time
}

func (e Entry) Updated() string {
	return e.updated.Format(time.RFC3339)
}

func fromPost(post db.Post, address string) Entry {
	return Entry{
		Title:   post.Title,
		ID:      fmt.Sprintf("%s/%d", address, post.ID),
		URL:     fmt.Sprintf("%s/%d", address, post.ID),
		updated: fromUnixEpoch(post.Created),
	}
}

func fromUnixEpoch(epoch int64) time.Time {
	return time.Unix(epoch, 0)
}
