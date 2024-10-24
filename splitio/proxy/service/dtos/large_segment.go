package dtos

import "net/http"

type LargeSegmentDTO struct {
	Name string
	Keys []string
}

type RfeDTO struct {
	Params       ParamsDTO `json:"p"`
	Format       int       `json:"f"`
	TotalKeys    int64     `json:"k"`
	Size         int64     `json:"s"`
	ChangeNumber int64     `json:"cn"`
	Name         string    `json:"n"`
	Version      string    `json:"v"`
}

type ParamsDTO struct {
	Method  string      `json:"m"`
	URL     string      `json:"u"`
	Headers http.Header `json:"h"`
}
