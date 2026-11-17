package server

import (
	"net/http"
	"strings"
)

// params holds the path parameters of a matched route. They are raw
// strings: every handler validates them before use.
type params map[string]string

type handlerFunc func(s *Server, w http.ResponseWriter, r *http.Request, p params)

// route is one entry of the route table. write marks the routes that
// change the notebook: they are refused in read-only mode, and every
// mutating method must be a write route.
type route struct {
	method  string
	pattern string
	segs    []string
	write   bool
	handler handlerFunc
}

func newRoute(method, pattern string, write bool, h handlerFunc) route {
	return route{method: method, pattern: pattern, segs: strings.Split(strings.Trim(pattern, "/"), "/"), write: write, handler: h}
}

// match finds the route for a request. When the path exists under other
// methods, rt is nil and allowed lists them.
func (s *Server) match(method, path string) (rt *route, p params, allowed []string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := range s.routes {
		c := &s.routes[i]
		pp, ok := c.matchPath(parts)
		if !ok {
			continue
		}
		if c.method == method || (method == http.MethodHead && c.method == http.MethodGet) {
			return c, pp, nil
		}
		allowed = append(allowed, c.method)
	}
	return nil, nil, allowed
}

func (c *route) matchPath(parts []string) (params, bool) {
	if len(parts) != len(c.segs) {
		return nil, false
	}
	var p params
	for i, seg := range c.segs {
		if strings.HasPrefix(seg, "{") {
			if p == nil {
				p = params{}
			}
			p[seg[1:len(seg)-1]] = parts[i]
			continue
		}
		if seg != parts[i] {
			return nil, false
		}
	}
	return p, true
}
