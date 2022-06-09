package server

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.seankhliao.com/svcrunner"
	"go.seankhliao.com/svcrunner/envflag"
	"go.seankhliao.com/webstyle"
	"go.seankhliao.com/webstyle/webstatic"
)

var (
	//go:embed index.md
	indexRaw []byte

	//go:embed repo.md.tpl
	repoRaw string
	repoTpl = template.Must(template.New("").Parse(repoRaw))

	headRaw = `
    <meta
      name="go-import"
      content="go.seankhliao.com/{{ .Repo }} git https://{{ .Source }}/{{ .Repo }}">
    <meta
      name="go-source"
      content="{{ .Host }}/{{ .Repo }}
        https://{{ .Source }}/{{ .Repo }}
        https://{{ .Source }}/{{ .Repo }}/tree/master{/dir}
        https://{{ .Source }}/{{ .Repo }}/blob/master{/dir}/{file}#L{line}">
`
	headTpl = template.Must(template.New("").Parse(headRaw))
)

type Server struct {
	host   string
	source string

	ts     time.Time
	render webstyle.Renderer

	indexOnce sync.Once
	index     []byte

	log   logr.Logger
	trace trace.Tracer
}

func New(hs *http.Server) *Server {
	s := &Server{
		ts:     time.Now(),
		render: webstyle.NewRenderer(webstyle.TemplateCompact),
	}
	mux := http.NewServeMux()
	mux.Handle("/", s)
	webstatic.Register(mux)
	hs.Handler = mux
	return s
}

func (s *Server) Register(c *envflag.Config) {
	c.StringVar(&s.host, "vanity.host", "go.seankhliao.com", "host this server runs on")
	c.StringVar(&s.source, "vanity.source", "github.com/seankhliao", "where the code is hosted")
}

func (s *Server) Init(ctx context.Context, t svcrunner.Tools) error {
	s.log = t.Log.WithName("vanity")
	s.trace = otel.Tracer("vanity")

	var err error
	s.index, err = s.render.RenderBytes(indexRaw, webstyle.Data{})
	if err != nil {
		return fmt.Errorf("render index page: %w", err)
	}
	return nil
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	log := s.log.WithValues("path", r.URL.Path)
	_, span := s.trace.Start(r.Context(), "serve")
	defer span.End()

	if r.Method != http.MethodGet {
		http.Error(rw, "GET only", http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" { // index
		http.ServeContent(rw, r, "index.html", s.ts, bytes.NewReader(s.index))
		log.V(1).Info("served index page")
		return
	}
	repo, _, _ := strings.Cut(p, "/")
	data := map[string]string{"Repo": repo, "Source": s.source, "Host": s.host}

	var buf1 bytes.Buffer
	err := repoTpl.Execute(&buf1, data)
	if err != nil {
		http.Error(rw, "render repo", http.StatusInternalServerError)
		log.Error(err, "render repotpl", "data", data)
		return
	}
	var buf2 bytes.Buffer
	err = headTpl.Execute(&buf2, data)
	if err != nil {
		http.Error(rw, "render head", http.StatusInternalServerError)
		log.Error(err, "render headtpl", "data", data)
		return
	}

	var buf3 bytes.Buffer
	err = s.render.Render(&buf3, &buf1, webstyle.Data{
		Head: buf2.String(),
	})
	if err != nil {
		http.Error(rw, "render html", http.StatusInternalServerError)
		log.Error(err, "render html")
		return
	}
	_, err = io.Copy(rw, &buf3)
	if err != nil {
		log.Error(err, "write response")
		return
	}
	log.V(1).Info("served repo page")
}
