package accoding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FileJournal is a conservative standalone journal for local validation.
// It stores only hashes and metadata, never code, cookies or passwords.
// A crashed lock is intentionally NOT removed automatically: inspect the
// directory and reconcile server records before manual recovery.
type FileJournal struct{ Directory string }
type entry struct {
	Attempt    Attempt     `json:"attempt"`
	Submission *Submission `json:"submission,omitempty"`
	Unknown    bool        `json:"unknown,omitempty"`
}

func (j FileJournal) locked(ctx context.Context, fn func() error) error {
	if !filepath.IsAbs(j.Directory) {
		return fail(Protocol, "记录目录必须是绝对路径")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := os.MkdirAll(j.Directory, 0700); err != nil {
		return err
	}
	lock := filepath.Join(j.Directory, ".lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return fail(Protocol, "提交记录被锁定；请先检查未决操作")
	}
	defer os.Remove(lock)
	return fn()
}
func writeNew(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		return fail(Protocol, "提交记录已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	if d, e := os.Open(filepath.Dir(path)); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
func readEntry(path string) (entry, error) {
	var e entry
	b, err := os.ReadFile(path)
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(b, &e)
	if err != nil || e.Attempt.Target.validate() != nil || !validID(string(e.Attempt.Account)) || len(e.Attempt.SourceHash) != 64 || e.Attempt.CreatedAt.IsZero() {
		return e, fail(Protocol, "提交记录损坏，拒绝发送")
	}
	return e, nil
}
func validKey(k string) bool {
	b, err := hex.DecodeString(k)
	return err == nil && len(b) == 16 && k == strings.ToLower(k)
}
func (j FileJournal) Begin(ctx context.Context, a Attempt) (key string, err error) {
	err = j.locked(ctx, func() error {
		if a.Target.validate() != nil || !validID(string(a.Account)) || len(a.SourceHash) != 64 || a.CreatedAt.IsZero() {
			return fail(Protocol, "提交记录参数无效")
		}
		files, err := os.ReadDir(j.Directory)
		if err != nil {
			return err
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".attempt.json") {
				continue
			}
			k := strings.TrimSuffix(f.Name(), ".attempt.json")
			if !validKey(k) {
				return fail(Protocol, "提交记录名称异常")
			}
			v, err := readEntry(filepath.Join(j.Directory, f.Name()))
			if err != nil {
				return err
			}
			if v.Attempt.Account != a.Account || v.Attempt.Target.ProblemID != a.Target.ProblemID {
				continue
			}
			resolved, err := readEntry(filepath.Join(j.Directory, k+".resolved.json"))
			if errors.Is(err, os.ErrNotExist) {
				return fail(Unknown, "此账号和题目存在未决提交，请核对后再操作")
			}
			if err != nil {
				return err
			}
			if !sameAttempt(resolved.Attempt, v.Attempt) || resolved.Submission == nil || resolved.Submission.verify(v.Attempt.Target, v.Attempt.Account) != nil {
				return fail(Protocol, "已确认记录损坏")
			}
		}
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		key = hex.EncodeToString(b[:])
		return writeNew(filepath.Join(j.Directory, key+".attempt.json"), entry{Attempt: a})
	})
	return
}
func (j FileJournal) Resolve(ctx context.Context, key string, s Submission) error {
	return j.finish(ctx, key, &s, false)
}
func (j FileJournal) Unknown(ctx context.Context, key string) error {
	return j.finish(ctx, key, nil, true)
}
func (j FileJournal) finish(ctx context.Context, key string, s *Submission, unknown bool) error {
	if !validKey(key) {
		return fail(Protocol, "无效提交记录标识")
	}
	return j.locked(ctx, func() error {
		e, err := readEntry(filepath.Join(j.Directory, key+".attempt.json"))
		if err != nil {
			return err
		}
		suffix := ".unknown.json"
		if s != nil {
			if err := s.verify(e.Attempt.Target, e.Attempt.Account); err != nil {
				return err
			}
			s.Detail = "" // Do not persist potentially sensitive judge detail.
			suffix = ".resolved.json"
		}
		e.Submission = s
		e.Unknown = unknown
		return writeNew(filepath.Join(j.Directory, key+suffix), e)
	})
}

func sameAttempt(a, b Attempt) bool {
	return a.Account == b.Account && a.Target == b.Target && a.Language == b.Language && a.SourceHash == b.SourceHash && a.CreatedAt.Equal(b.CreatedAt)
}
