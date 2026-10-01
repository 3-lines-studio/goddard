package app

import "net/http"

func Load(r *http.Request) (any, error) {
	return map[string]string{}, nil
}
