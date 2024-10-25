package dtos

import "net/http"

type LargeSegmentResponse struct {
	Error error
	Retry bool
	Data  *LargeSegmentDTO
}

type LargeSegmentDTO struct {
	Name         string
	Keys         []string
	ChangeNumber int64
}

type RfeDTO struct {
	Params       ParamsDTO `json:"p"`
	Interval     *int64    `json:"i,omitempty"` // interval
	Format       int       `json:"f"`
	TotalKeys    int64     `json:"k"`
	Size         int64     `json:"s"`
	ChangeNumber int64     `json:"cn"`
	Name         string    `json:"n"`
	Version      string    `json:"v"`
	ExpiresAt    int64     `json:"e"`
}

type ParamsDTO struct {
	Method  string      `json:"m"`
	URL     string      `json:"u"`
	Headers http.Header `json:"h,omitempty"`
	Body    []byte      `json:"b,omitempty"`
}
