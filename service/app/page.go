package service

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func Load(r *http.Request) (any, error) {
	return bifrost.PageData{
		Props:    map[string]string{"title": "Goddard"},
		Document: bifrost.Document{Lang: "es"},
	}, nil
}
