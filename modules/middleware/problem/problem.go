package problem

import (
	"encoding/json"
	"net/http"
)

// Problem is an RFC7807 Problem Details document with optional extensions.
// It mirrors the OpenAPI Problem schema used across services.
type Problem struct {
	Code          *string         `json:"code,omitempty"`
	Detail        *string         `json:"detail,omitempty"`
	Instance      *string         `json:"instance,omitempty"`
	InvalidParams *[]InvalidParam `json:"invalidParams,omitempty"`
	Status        int             `json:"status"`
	Title         string          `json:"title"`
	TraceID       *string         `json:"traceId,omitempty"`
	Type          *string         `json:"type,omitempty"`

	// Extensions holds additional non-standard fields.
	Extensions map[string]any `json:"-"`
}

type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type Option func(*Problem)

func New(opts ...Option) *Problem {
	p := &Problem{
		Type:   strPtr("about:blank"),
		Title:  http.StatusText(http.StatusInternalServerError),
		Status: http.StatusInternalServerError,
		Detail: strPtr("unhandled error"),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	if p.Type == nil {
		p.Type = strPtr("about:blank")
	}
	if p.Title == "" {
		if t := http.StatusText(p.Status); t != "" {
			p.Title = t
		} else {
			p.Title = "Unknown Error"
		}
	}
	return p
}

func Write(w http.ResponseWriter, p *Problem) {
	if p == nil {
		p = Internal("server error")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func WithStatus(status int) Option {
	return func(p *Problem) { p.Status = status }
}

func WithTitle(title string) Option {
	return func(p *Problem) { p.Title = title }
}

func WithDetail(detail string) Option {
	return func(p *Problem) { p.Detail = strPtr(detail) }
}

func WithType(typ string) Option {
	return func(p *Problem) { p.Type = strPtr(typ) }
}

func WithCode(code string) Option {
	return func(p *Problem) { p.Code = strPtr(code) }
}

func WithTraceID(traceID string) Option {
	return func(p *Problem) { p.TraceID = strPtr(traceID) }
}

func WithInvalidParam(name, reason string) Option {
	return func(p *Problem) {
		if p.InvalidParams == nil {
			s := []InvalidParam{{Name: name, Reason: reason}}
			p.InvalidParams = &s
			return
		}
		s := append(*p.InvalidParams, InvalidParam{Name: name, Reason: reason})
		p.InvalidParams = &s
	}
}

func WithExtension(key string, value any) Option {
	return func(p *Problem) {
		if p.Extensions == nil {
			p.Extensions = map[string]any{}
		}
		p.Extensions[key] = value
	}
}

func BadRequest(detail string, opts ...Option) *Problem {
	base := []Option{
		WithTitle("Bad Request"),
		WithStatus(http.StatusBadRequest),
		WithDetail(detail),
	}
	return New(append(base, opts...)...)
}

func MethodNotAllowed(detail string, opts ...Option) *Problem {
	base := []Option{
		WithTitle("Method Not Allowed"),
		WithStatus(http.StatusMethodNotAllowed),
		WithDetail(detail),
	}
	return New(append(base, opts...)...)
}

func TooManyRequests(detail string, opts ...Option) *Problem {
	base := []Option{
		WithTitle("Too Many Requests"),
		WithStatus(http.StatusTooManyRequests),
		WithDetail(detail),
	}
	return New(append(base, opts...)...)
}

func Internal(detail string, opts ...Option) *Problem {
	base := []Option{
		WithTitle("Internal Server Error"),
		WithStatus(http.StatusInternalServerError),
		WithDetail(detail),
	}
	return New(append(base, opts...)...)
}

func strPtr(s string) *string { return &s }

// MarshalJSON merges Extensions into the base Problem object.
func (p Problem) MarshalJSON() ([]byte, error) {
	// create a new type with the same fields but without Problem's method
	// since encoding/json would see that Problem has a MarshalJSON method
	// and would call it again -> infinite recursion
	//
	// So basically: is it the standard Go pattern to “bypass my own
	// custom marshaler and serialize the plain struct first,” then
	// we can layer custom behavior (merging extensions) on top.
	// TODO: encoding/json/v2
	// TODO: note on encoding/json behavior here
	type alias Problem
	base, err := json.Marshal(alias(p))
	if err != nil {
		return nil, err
	}
	if len(p.Extensions) == 0 {
		return base, nil
	}
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	for k, v := range p.Extensions {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return json.Marshal(m)
}
