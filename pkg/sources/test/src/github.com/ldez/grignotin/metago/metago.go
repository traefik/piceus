// Package metago A client to get meta information related to go-get.
// go-import and go-source.
package metago

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// MetaGo information from the meta tags
type MetaGo struct {
	Pkg      string
	GoSource []string
	GoImport []string
}

// Get gets go-get meta information from the meta tags.
func Get(moduleName string) (*MetaGo, error) {
	req, err := http.NewRequest(http.MethodGet, makeURL(moduleName), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	meta, err := parseMetaGo(resp.Body)
	if err != nil {
		// with HTML5, some <script> content are not XML valid.
		var e *xml.SyntaxError
		if errors.As(err, &e) {
			return &MetaGo{Pkg: moduleName}, nil
		}

		return nil, err
	}

	if meta != nil {
		meta.Pkg = moduleName
	}

	return meta, err
}

func makeURL(moduleName string) string {
	name := moduleName
	exp := regexp.MustCompile(`(.+/.+)/v\d+$`)
	if exp.MatchString(moduleName) {
		name = exp.FindStringSubmatch(moduleName)[1]
	}

	return "https://" + name + "?go-get=1"
}

func parseMetaGo(r io.Reader) (*MetaGo, error) {
	decoder := xml.NewDecoder(r)
	decoder.CharsetReader = charsetReader
	decoder.Strict = false

	meta := &MetaGo{}

	for {
		token, err := decoder.RawToken()
		if err != nil {
			if err != io.EOF && len(meta.GoSource) == 0 && len(meta.GoImport) == 0 {
				return nil, err
			}
			break
		}

		if e, ok := token.(xml.StartElement); ok && strings.EqualFold(e.Name.Local, "body") {
			break
		}
		if e, ok := token.(xml.EndElement); ok && strings.EqualFold(e.Name.Local, "head") {
			break
		}

		e, ok := token.(xml.StartElement)
		if !ok || !strings.EqualFold(e.Name.Local, "meta") {
			continue
		}

		switch attrValue(e.Attr, "name") {
		case "go-import":
			meta.GoImport = strings.Fields(attrValue(e.Attr, "content"))
		case "go-source":
			meta.GoSource = strings.Fields(attrValue(e.Attr, "content"))
		default:
			continue
		}
	}

	return meta, nil
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "ascii":
		return input, nil
	default:
		return nil, fmt.Errorf("can't decode XML document using charset %q", charset)
	}
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

// EffectivePkgSource get effective source package.
func EffectivePkgSource(m *MetaGo) string {
	if m == nil {
		return ""
	}

	if len(m.GoSource) > 0 {
		a := m.GoSource[len(m.GoSource)-1]
		split := strings.Split(a, "/")
		return strings.Join(split[2:5], "/")
	}

	if len(m.GoImport) == 0 {
		return m.Pkg
	}

	return m.GoImport[0]
}
