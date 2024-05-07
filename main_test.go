package main

import (
	"fmt"
	"net/url"
	"testing"
)

func TestGetIds(t *testing.T) {
	type setup struct {
		path   string
		expIds []int
		expErr error
	}
	setups := []setup{
		{path: "/"},
		{path: "/1", expIds: []int{1}},
		{path: "/100", expIds: []int{100}},
		{path: "/1000000000000000000000000000000000000000000", expErr: fmt.Errorf("strconv.Atoi error")},
		{path: "/foo", expErr: fmt.Errorf("Some error")},
	}

	for _, s := range setups {
		u := &url.URL{Path: s.path}

		ids, err := getIds(u)
		if err != nil && s.expErr == nil {
			t.Errorf("expected no error, got %s", err)
		} else if err == nil && s.expErr != nil {
			t.Errorf("expected error %s, got nothing", s.expErr)
		}

		if len(ids) != len(s.expIds) {
			t.Errorf("expected %d IDs, got %d IDs", len(s.expIds), len(ids))
		}

		for i, id := range ids {
			if s.expIds[i] != id {
				t.Errorf("expected %d, got %d", s.expIds[i], id)
			}
		}
	}

}
