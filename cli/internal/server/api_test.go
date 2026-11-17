package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReads(t *testing.T) {
	e := newEnv(t, false)

	info := e.json("GET", "/api/v1/info", nil, 200)
	if info["name"] != "Example role" || info["language"] != "en" || info["format"] != float64(1) || info["read_only"] != false {
		t.Errorf("info: %v", info)
	}
	if _, has := info["path"]; has {
		t.Error("info exposes a path")
	}

	secs := e.json("GET", "/api/v1/sections", nil, 200)["sections"].([]any)
	if len(secs) != 2 {
		t.Fatalf("sections: %v", secs)
	}
	first := secs[0].(map[string]any)
	if first["slug"] != "technical" || first["version"] == "" || first["counts"].(map[string]any)["total"] != float64(3) {
		t.Errorf("section: %v", first)
	}

	sec := e.json("GET", "/api/v1/sections/technical", nil, 200)
	if sec["title"] != "Technical" || sec["description"] != "Systems and tools." {
		t.Errorf("section: %v", sec)
	}
	if rec := e.do("GET", "/api/v1/sections/nope", nil); rec.Code != 404 || errCode(t, rec) != "not_found" {
		t.Errorf("missing section: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/api/v1/sections/Bad_Slug", nil); rec.Code != 400 || errCode(t, rec) != "invalid_slug" {
		t.Errorf("bad slug: %d %s", rec.Code, rec.Body)
	}

	topics := e.json("GET", "/api/v1/sections/technical/topics", nil, 200)["topics"].([]any)
	if len(topics) != 3 || topics[0].(map[string]any)["ref"] != "technical/backtesting" {
		t.Errorf("topics: %v", topics)
	}
	outline := e.json("GET", "/api/v1/outline", nil, 200)["outline"].([]any)
	if len(outline) != 2 || len(outline[0].(map[string]any)["topics"].([]any)) != 3 {
		t.Errorf("outline: %v", outline)
	}

	tp := e.json("GET", "/api/v1/topics/technical/backtesting", nil, 200)
	if tp["title"] != "Backtesting" || tp["status"] != "in-progress" || tp["section_title"] != "Technical" ||
		tp["notes"] != "Download the sample data first." || str(tp, "version") == "" {
		t.Errorf("topic: %v", tp)
	}
	if rs := tp["resources"].([]any); len(rs) != 1 || rs[0].(map[string]any)["url"] != "https://example.com/wiki" {
		t.Errorf("resources: %v", tp["resources"])
	}
	if rec := e.do("GET", "/api/v1/topics/technical/nope", nil); rec.Code != 404 {
		t.Errorf("missing topic: %d", rec.Code)
	}

	j := e.json("GET", "/api/v1/journal", nil, 200)
	es := j["entries"].([]any)
	if len(es) != 3 || j["has_more"] != false {
		t.Fatalf("journal: %v", j)
	}
	if es[2].(map[string]any)["date"] != "2026-10-20" { // newest first
		t.Errorf("order: %v", es)
	}
	for q, want := range map[string]int{
		"from=2026-11-01": 2, "to=2026-10-31": 1, "section=team": 1, "section=technical": 1,
		"topic=technical/backtesting": 1, "from=2026-11-03&to=2026-11-03": 2,
	} {
		got := e.json("GET", "/api/v1/journal?"+q, nil, 200)["entries"].([]any)
		if len(got) != want {
			t.Errorf("journal?%s: %d entries, want %d", q, len(got), want)
		}
	}
	page := e.json("GET", "/api/v1/journal?limit=2", nil, 200)
	if len(page["entries"].([]any)) != 2 || page["has_more"] != true {
		t.Errorf("page 1: %v", page)
	}
	page = e.json("GET", "/api/v1/journal?limit=2&offset=2", nil, 200)
	if len(page["entries"].([]any)) != 1 || page["has_more"] != false {
		t.Errorf("page 2: %v", page)
	}
	for _, q := range []string{"from=bad", "to=2026-13-01", "section=BAD", "topic=nope", "limit=0", "limit=100000", "limit=x", "offset=-1"} {
		if rec := e.do("GET", "/api/v1/journal?"+q, nil); rec.Code != 400 {
			t.Errorf("journal?%s: %d", q, rec.Code)
		}
	}
	id := es[0].(map[string]any)["id"].(string)
	ent := e.json("GET", "/api/v1/journal/"+id, nil, 200)
	if ent["id"] != id || str(ent, "version") == "" {
		t.Errorf("entry: %v", ent)
	}
	if rec := e.do("GET", "/api/v1/journal/0000000000", nil); rec.Code != 404 {
		t.Errorf("missing entry: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/journal/not-an-id", nil); rec.Code != 400 {
		t.Errorf("bad entry id: %d", rec.Code)
	}

	steps := e.json("GET", "/api/v1/role-path", nil, 200)["steps"].([]any)
	if len(steps) != 3 || steps[0].(map[string]any)["step"] != float64(1) {
		t.Errorf("role path: %v", steps)
	}
	pr := e.json("GET", "/api/v1/progress", nil, 200)
	if pr["all"].(map[string]any)["total"] != float64(4) || pr["journal_entries"] != float64(3) || pr["suggested"] != float64(1) {
		t.Errorf("progress: %v", pr)
	}

	hits := e.json("GET", "/api/v1/search?q=simulator", nil, 200)["hits"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["kind"] != "entry" {
		t.Errorf("search: %v", hits)
	}
	for _, q := range []string{"", "%00", strings.Repeat("a", 300)} {
		if rec := e.do("GET", "/api/v1/search?q="+q, nil); rec.Code != 400 || errCode(t, rec) != "invalid_query" {
			t.Errorf("search %q: %d %s", q, rec.Code, rec.Body)
		}
	}
	if rec := e.do("GET", "/api/v1/search?q=a&limit=999", nil); rec.Code != 400 {
		t.Errorf("search limit: %d", rec.Code)
	}
	// HEAD works on reads and sends no body.
	if rec := e.do("HEAD", "/api/v1/info", nil); rec.Code != 200 {
		t.Errorf("HEAD: %d", rec.Code)
	}
	// The response is JSON, never sniffable HTML.
	rec := e.do("GET", "/api/v1/info", nil)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
}

// Titles with HTML come back as plain data, with the HTML characters
// escaped in the JSON so that they can never be taken for markup.
func TestHTMLInContentIsEscapedInJSON(t *testing.T) {
	e := newEnv(t, false)
	e.json("POST", "/api/v1/sections", map[string]any{"title": "<img src=x onerror=alert(1)>"}, 201)
	rec := e.do("GET", "/api/v1/sections", nil)
	if strings.Contains(rec.Body.String(), "<img") {
		t.Errorf("raw HTML in response: %s", rec.Body)
	}
	var out struct {
		Sections []struct{ Title string } `json:"sections"`
	}
	if err := jsonUnmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range out.Sections {
		found = found || s.Title == "<img src=x onerror=alert(1)>"
	}
	if !found {
		t.Errorf("title lost: %s", rec.Body)
	}
}

func TestWritesRoundTrip(t *testing.T) {
	e := newEnv(t, false)

	// Sections.
	sec := e.json("POST", "/api/v1/sections", map[string]any{"title": "Operations", "description": "Day to day."}, 201)
	if sec["slug"] != "operations" || sec["order"] != float64(2) {
		t.Fatalf("add section: %v", sec)
	}
	if rec := e.do("POST", "/api/v1/sections", map[string]any{"title": "Dup", "slug": "operations"}); rec.Code != 409 || errCode(t, rec) != "already_exists" {
		t.Errorf("duplicate slug: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/v1/sections", map[string]any{"title": ""}); rec.Code != 422 || errCode(t, rec) != "empty" {
		t.Errorf("empty title: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/v1/sections", map[string]any{"title": strings.Repeat("x", 200)}); rec.Code != 422 || errCode(t, rec) != "too_long" {
		t.Errorf("long title: %d %s", rec.Code, rec.Body)
	}
	upd := e.json("PATCH", "/api/v1/sections/operations", map[string]any{"version": sec["version"], "title": "Ops", "description": "Changed."}, 200)
	if upd["title"] != "Ops" || upd["description"] != "Changed." || upd["version"] == sec["version"] {
		t.Errorf("update section: %v", upd)
	}
	mv := e.json("POST", "/api/v1/sections/operations/move", map[string]any{"to": "ops"}, 200)
	if mv["slug"] != "ops" {
		t.Errorf("move section: %v", mv)
	}
	if got := e.json("POST", "/api/v1/sections/ops/reorder", map[string]any{"delta": -1}, 200); got["moved"] != true {
		t.Errorf("reorder section: %v", got)
	}
	if got := e.json("POST", "/api/v1/sections/technical/reorder", map[string]any{"delta": -1}, 200); got["moved"] != false {
		t.Errorf("reorder section at the edge: %v", got)
	}
	for _, d := range []any{0, 2, -2, "x"} {
		if rec := e.do("POST", "/api/v1/sections/ops/reorder", map[string]any{"delta": d}); rec.Code != 400 {
			t.Errorf("delta %v: %d", d, rec.Code)
		}
	}

	// Topics.
	tp := e.json("POST", "/api/v1/sections/ops/topics", map[string]any{
		"title": "Deploys", "type": "role", "notes": "Run the script.",
		"resources": []map[string]string{{"url": "https://example.com/deploys", "title": "Runbook"}},
	}, 201)
	if tp["ref"] != "ops/deploys" || tp["status"] != "pending" {
		t.Fatalf("add topic: %v", tp)
	}
	if rec := e.do("POST", "/api/v1/sections/ops/topics", map[string]any{"title": "Bad", "status": "done"}); rec.Code != 422 {
		t.Errorf("bad status: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/v1/sections/ops/topics", map[string]any{"title": "Bad", "resources": []map[string]string{{"url": "javascript:alert(1)"}}}); rec.Code != 422 {
		t.Errorf("javascript: url accepted: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/v1/sections/nope/topics", map[string]any{"title": "X"}); rec.Code != 404 {
		t.Errorf("topic in missing section: %d", rec.Code)
	}
	tu := e.json("PATCH", "/api/v1/topics/ops/deploys", map[string]any{"version": tp["version"], "title": "Deploying", "notes": "New notes", "suggested": true}, 200)
	if tu["title"] != "Deploying" || tu["notes"] != "New notes" || tu["suggested"] != true || len(tu["resources"].([]any)) != 1 {
		t.Errorf("update topic (partial update must keep the rest): %v", tu)
	}
	st := e.json("POST", "/api/v1/topics/ops/deploys/status", map[string]any{"status": "mastered"}, 200)
	if st["status"] != "mastered" {
		t.Errorf("status: %v", st)
	}
	if rec := e.do("POST", "/api/v1/topics/ops/deploys/status", map[string]any{"status": "nope"}); rec.Code != 422 {
		t.Errorf("bad status: %d", rec.Code)
	}
	e.json("POST", "/api/v1/topics/ops/deploys/status", map[string]any{"status": "pending"}, 200)
	adv := e.json("POST", "/api/v1/topics/ops/deploys/advance", nil, 200)
	if adv["status"] != "in-progress" {
		t.Errorf("advance: %v", adv)
	}
	adv = e.json("POST", "/api/v1/topics/ops/deploys/advance", map[string]any{"version": adv["version"]}, 200)
	if adv["status"] != "mastered" {
		t.Errorf("advance: %v", adv)
	}
	adv = e.json("POST", "/api/v1/topics/ops/deploys/advance", nil, 200)
	if adv["status"] != "mastered" {
		t.Errorf("advance past mastered: %v", adv)
	}
	if got := e.json("POST", "/api/v1/topics/technical/risk/reorder", map[string]any{"delta": -1}, 200); got["moved"] != true {
		t.Errorf("reorder topic: %v", got)
	}
	mt := e.json("POST", "/api/v1/topics/ops/deploys/move", map[string]any{"to": "team/releases"}, 200)
	if mt["ref"] != "team/releases" {
		t.Errorf("move topic: %v", mt)
	}

	// Journal.
	en := e.json("POST", "/api/v1/journal", map[string]any{"body": "Learned the deploy steps.", "topic": "team/releases", "date": "2026-11-04"}, 201)
	if en["date"] != "2026-11-04" || en["section"] != "team" || en["topic"] != "team/releases" {
		t.Fatalf("add entry: %v", en)
	}
	if rec := e.do("POST", "/api/v1/journal", map[string]any{"body": "x", "topic": "team/nope"}); rec.Code != 404 {
		t.Errorf("entry on a missing topic: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/v1/journal", map[string]any{"body": ""}); rec.Code != 422 {
		t.Errorf("empty entry: %d %s", rec.Code, rec.Body)
	}
	id := en["id"].(string)
	eu := e.json("PATCH", "/api/v1/journal/"+id, map[string]any{"version": en["version"], "body": "Edited.", "topic": ""}, 200)
	if eu["body"] != "Edited." || eu["topic"] != "" || eu["section"] != "team" {
		t.Errorf("update entry: %v", eu)
	}
	e.json("DELETE", "/api/v1/journal/"+id, nil, 204)
	if rec := e.do("GET", "/api/v1/journal/"+id, nil); rec.Code != 404 {
		t.Errorf("entry still there: %d", rec.Code)
	}

	// Deletes.
	e.json("DELETE", "/api/v1/topics/team/releases", nil, 204)
	if rec := e.do("DELETE", "/api/v1/topics/team/releases", nil); rec.Code != 404 {
		t.Errorf("delete twice: %d", rec.Code)
	}
	e.json("DELETE", "/api/v1/sections/ops", nil, 204)
	if rec := e.do("GET", "/api/v1/sections/ops", nil); rec.Code != 404 {
		t.Errorf("section still there: %d", rec.Code)
	}
}

func TestConflicts(t *testing.T) {
	e := newEnv(t, false)

	type kind struct {
		name, get, patch string
		body             map[string]any
		file             string
	}
	for _, k := range []kind{
		{"section", "/api/v1/sections/team", "/api/v1/sections/team", map[string]any{"title": "Teams"}, "sections/team/_section.md"},
		{"topic", "/api/v1/topics/technical/risk", "/api/v1/topics/technical/risk", map[string]any{"title": "Risks"}, "sections/technical/risk.md"},
	} {
		t.Run(k.name, func(t *testing.T) {
			cur := e.json("GET", k.get, nil, 200)
			v1 := cur["version"].(string)
			body := func(v string) map[string]any {
				m := map[string]any{"version": v}
				for a, b := range k.body {
					m[a] = b
				}
				return m
			}
			// Another program changes the file after the client read it.
			path := filepath.Join(e.dir, filepath.FromSlash(k.file))
			data := readFile(t, path)
			must(t, os.WriteFile(path, []byte(strings.Replace(data, "updated:", "updated:", 1)+"\n"), 0o600))
			rec := e.do("PATCH", k.patch, body(v1))
			if rec.Code != http.StatusConflict || errCode(t, rec) != "conflict" {
				t.Fatalf("stale version after an outside change: %d %s", rec.Code, rec.Body)
			}
			// Read again, update, and the old version is stale now.
			cur = e.json("GET", k.get, nil, 200)
			v2 := cur["version"].(string)
			if v2 == v1 {
				t.Fatal("version did not change")
			}
			e.json("PATCH", k.patch, body(v2), 200)
			rec = e.do("PATCH", k.patch, body(v2))
			if rec.Code != 409 {
				t.Errorf("second update with the same version: %d %s", rec.Code, rec.Body)
			}
		})
	}

	t.Run("entry", func(t *testing.T) {
		es := e.json("GET", "/api/v1/journal", nil, 200)["entries"].([]any)
		first := es[0].(map[string]any)
		id, v := first["id"].(string), first["version"].(string)
		e.json("PATCH", "/api/v1/journal/"+id, map[string]any{"version": v, "body": "One"}, 200)
		rec := e.do("PATCH", "/api/v1/journal/"+id, map[string]any{"version": v, "body": "Two"})
		if rec.Code != 409 || errCode(t, rec) != "conflict" {
			t.Errorf("stale entry update: %d %s", rec.Code, rec.Body)
		}
		if got := e.json("GET", "/api/v1/journal/"+id, nil, 200); got["body"] != "One" {
			t.Errorf("stale update changed the entry: %v", got)
		}
	})

	t.Run("status with a stale version", func(t *testing.T) {
		cur := e.json("GET", "/api/v1/topics/team/rituals", nil, 200)
		e.json("POST", "/api/v1/topics/team/rituals/status", map[string]any{"status": "in-progress"}, 200)
		rec := e.do("POST", "/api/v1/topics/team/rituals/status", map[string]any{"status": "mastered", "version": cur["version"]})
		if rec.Code != 409 {
			t.Errorf("%d %s", rec.Code, rec.Body)
		}
	})

	t.Run("bad versions", func(t *testing.T) {
		for _, v := range []any{nil, "", "x", "1-zz", "-1-" + strings.Repeat("0", 64), "1-" + strings.Repeat("0", 63), 5} {
			m := map[string]any{"title": "T"}
			if v != nil {
				m["version"] = v
			}
			rec := e.do("PATCH", "/api/v1/sections/team", m)
			if rec.Code != 400 {
				t.Errorf("version %v: %d %s", v, rec.Code, rec.Body)
			}
		}
		rec := e.do("PATCH", "/api/v1/sections/team", map[string]any{"title": "T", "version": "1-" + strings.Repeat("0", 64)})
		if rec.Code != 409 {
			t.Errorf("well-formed but wrong version: %d", rec.Code)
		}
	})
}

func TestConcurrentWritesAreSerialized(t *testing.T) {
	e := newEnv(t, false)
	done := make(chan int, 20)
	for i := range 20 {
		go func() {
			rec := e.do("POST", "/api/v1/journal", map[string]any{"body": fmt.Sprintf("entry %d", i), "date": "2026-11-05"})
			done <- rec.Code
		}()
	}
	for range 20 {
		if c := <-done; c != 201 {
			t.Errorf("status %d", c)
		}
	}
	es := e.json("GET", "/api/v1/journal?from=2026-11-05&limit=100", nil, 200)["entries"].([]any)
	if len(es) != 20 {
		t.Errorf("%d entries written", len(es))
	}
}
