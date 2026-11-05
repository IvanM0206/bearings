package gitsync

import (
	"strings"
	"testing"
)

func TestClassifyKey(t *testing.T) {
	cases := map[string]keyClass{
		"core.fsmonitor":                          keyNeutralized,
		"core.hookspath":                          keyNeutralized,
		"core.sshcommand":                         keyRefused,
		"core.askpass":                            keyRefused,
		"core.gitproxy":                           keyRefused,
		"core.alternaterefscommand":               keyRefused,
		"core.worktree":                           keyRefused,
		"credential.helper":                       keyRefused,
		"credential.https://example.com.helper":   keyRefused,
		"credential.username":                     keyOK,
		"filter.lfs.clean":                        keyRefused,
		"filter.lfs.smudge":                       keyRefused,
		"filter.lfs.process":                      keyRefused,
		"filter.lfs.required":                     keyOK,
		"diff.external":                           keyRefused,
		"diff.x.textconv":                         keyRefused,
		"diff.x.command":                          keyRefused,
		"diff.renames":                            keyOK,
		"merge.x.driver":                          keyRefused,
		"gpg.program":                             keyRefused,
		"gpg.ssh.program":                         keyRefused,
		"gpg.x509.program":                        keyRefused,
		"gpg.ssh.defaultkeycommand":               keyRefused,
		"gpg.format":                              keyOK,
		"remote.origin.uploadpack":                keyRefused,
		"remote.origin.receivepack":               keyRefused,
		"remote.origin.proxy":                     keyRefused,
		"remote.origin.url":                       keyOK,
		"remote.origin.fetch":                     keyOK,
		"protocol.allow":                          keyRefused,
		"protocol.ext.allow":                      keyRefused,
		"url.https://a.example/.insteadof":        keyRefused,
		"url.git@a.example:.pushinsteadof":        keyRefused,
		"http.proxy":                              keyRefused,
		"http.sslverify":                          keyRefused,
		"http.https://a.example/.sslverify":       keyRefused,
		"http.https://a.example/.extraheader":     keyRefused,
		"http.postbuffer":                         keyOK,
		"user.name":                               keyOK,
		"user.email":                              keyOK,
		"branch.main.remote":                      keyOK,
		"core.repositoryformatversion":            keyOK,
		"CORE.SSHCOMMAND":                         keyRefused,
		"nodot":                                   keyOK,
		"include.path":                            keyOK, // includes are followed and checked
		"includeif.gitdir:/x/.path":               keyOK,
		"submodule.recurse":                       keyOK, // overridden on the command line
		"sequence.editor":                         keyOK, // overridden by GIT_SEQUENCE_EDITOR
		"core.editor":                             keyOK, // overridden by GIT_EDITOR
		"core.pager":                              keyOK, // --no-pager
		"credential.https://a.example/x.y.helper": keyRefused,
	}
	for key, want := range cases {
		if got := classifyKey(key); got != want {
			t.Errorf("classifyKey(%q) = %d, want %d", key, got, want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string][2]int{
		"2.43.0":                 {2, 43},
		"2.39.3 (Apple Git-145)": {2, 39},
		"2.42.0.windows.2":       {2, 42},
		"2.26":                   {2, 26},
		"3.0.0-rc1":              {3, 0},
	} {
		ma, mi, ok := parseVersion(in)
		if !ok || ma != want[0] || mi != want[1] {
			t.Errorf("parseVersion(%q) = %d, %d, %v", in, ma, mi, ok)
		}
	}
	for _, in := range []string{"", "two.three", "2", "x.1"} {
		if _, _, ok := parseVersion(in); ok {
			t.Errorf("parseVersion(%q) accepted", in)
		}
	}
}

func TestEnvironment(t *testing.T) {
	r := New("/usr/bin/git", "/nb", []string{
		"PATH=/usr/bin", "HOME=/home/u", "GIT_DIR=/elsewhere/.git", "git_work_tree=/x", "GIT_INDEX_FILE=/x",
		"GIT_EDITOR=vim", "GIT_GLOB_PATHSPECS=1", "GIT_SSH_COMMAND=ssh -i key", "GIT_TERMINAL_PROMPT=1",
	}, false)
	env := strings.Join(r.env, "\n")
	for _, bad := range []string{"GIT_DIR=", "git_work_tree=", "GIT_INDEX_FILE=", "GIT_EDITOR=vim", "GIT_GLOB_PATHSPECS", "GIT_TERMINAL_PROMPT=1"} {
		if strings.Contains(env, bad) {
			t.Errorf("%s passed to git", bad)
		}
	}
	for _, good := range []string{"PATH=/usr/bin", "HOME=/home/u", "GIT_SSH_COMMAND=ssh -i key", "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0"} {
		if !strings.Contains(env, good) {
			t.Errorf("%s missing", good)
		}
	}
	if strings.Contains(strings.Join(New("git", "/nb", nil, true).env, "\n"), "GIT_TERMINAL_PROMPT") {
		t.Error("interactive runs must let git prompt")
	}
}

func TestHardeningArguments(t *testing.T) {
	joined := strings.Join(hardening, " ")
	for _, want := range []string{"core.fsmonitor=false", "core.hooksPath=/dev/null", "protocol.ext.allow=never", "protocol.file.allow=user", "submodule.recurse=false"} {
		if !strings.Contains(joined, "-c "+want) {
			t.Errorf("missing -c %s", want)
		}
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &limitedBuffer{max: 4}
	n, err := b.Write([]byte("abcdef"))
	if n != 6 || err != nil || b.buf.String() != "abcd" || !b.truncated {
		t.Fatalf("%d %v %q %v", n, err, b.buf.String(), b.truncated)
	}
}
