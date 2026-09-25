package api

import (
	"net/http"
	"strconv"
)

type ErrorResponse struct {
	Errors []Error `json:"errors"`
}

type Error struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func NewError(status int, detail string) Error {
	return Error{
		Status: strconv.Itoa(status),
		Title:  http.StatusText(status),
		Detail: detail,
	}
}
