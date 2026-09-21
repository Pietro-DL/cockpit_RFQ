package web_test

import (
	"io/fs"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/transport/web"
)

func TestTemplates(t *testing.T) {
	templ, err := fs.Sub(risorse.FS, "web/templates")
	if err != nil {
		t.Fatalf("sub fs: %v", err)
	}
	static, err := fs.Sub(risorse.FS, "web/static")
	if err != nil {
		t.Fatalf("sub fs: %v", err)
	}

	s := &web.Server{Templ: templ, Static: static}
	if err := s.Init(); err != nil {
		t.Fatalf("Init templates fallito: %v", err)
	}
}
