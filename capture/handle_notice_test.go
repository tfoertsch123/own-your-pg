package capture

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/tfoertsch123/log"
)

func newNoticeTestSession(t *testing.T, ignMissId []string) *session {
	t.Helper()
	lg := log.NewR(log.WithTopic("TEST"))
	return &session{
		lg:           lg,
		mlg:          lg.New(log.WithTopic("WRT")),
		ignMissIdMap: makeMap(ignMissId),
	}
}

func notice(sev, message string) *pgproto3.NoticeResponse {
	return &pgproto3.NoticeResponse{
		SeverityUnlocalized: sev,
		Message:             message,
	}
}

func TestHandleNotice_ErrorSeverity(t *testing.T) {
	for _, sev := range []string{"ERROR", "FATAL", "PANIC"} {
		t.Run(sev, func(t *testing.T) {
			s := newNoticeTestSession(t, nil)
			err := s.handleNotice(notice(sev, "something bad"))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, ErrNoticeMessage) {
				t.Errorf("error should wrap ErrNoticeMessage: %v", err)
			}
			if !strings.Contains(err.Error(), sev) {
				t.Errorf("error should contain severity %q: %v", sev, err)
			}
			if !strings.Contains(err.Error(), "something bad") {
				t.Errorf("error should contain message: %v", err)
			}
		})
	}
}

func TestHandleNotice_WarningNoReplidMatch(t *testing.T) {
	s := newNoticeTestSession(t, nil)
	// A WARNING that does not match the missing-identity pattern must
	// be ignored.
	if err := s.handleNotice(notice("WARNING",
		"some unrelated warning")); err != nil {
		t.Errorf("unrelated WARNING should be ignored, got %v", err)
	}
}

func TestHandleNotice_OtherSeveritiesIgnored(t *testing.T) {
	for _, sev := range []string{"DEBUG", "INFO", "NOTICE", "LOG"} {
		t.Run(sev, func(t *testing.T) {
			s := newNoticeTestSession(t, nil)
			if err := s.handleNotice(notice(sev, "fyi")); err != nil {
				t.Errorf("severity %s should be ignored, got %v", sev, err)
			}
		})
	}
}

func TestHandleNotice_MissingIdentity_Ignored(t *testing.T) {
	const noticeMsg = `no tuple identifier for UPDATE in table "public"."orders"`
	s := newNoticeTestSession(t, nil)
	if err := s.handleNotice(notice("WARNING", noticeMsg)); err == nil {
		t.Error("missing identity without ignore list must be an error")
	} else if !errors.Is(err, ErrNoticeMessage) {
		t.Errorf("error should wrap ErrNoticeMessage: %v", err)
	}
}

func TestHandleNotice_MissingIdentity_IgnoreAll(t *testing.T) {
	const noticeMsg = `no tuple identifier for UPDATE in table "public"."orders"`
	s := newNoticeTestSession(t, []string{"*"})
	if err := s.handleNotice(notice("WARNING", noticeMsg)); err != nil {
		t.Errorf("ignore-all should silence the notice, got %v", err)
	}
}

func TestHandleNotice_MissingIdentity_IgnoreSchema(t *testing.T) {
	const noticeMsg = `no tuple identifier for UPDATE in table "public"."orders"`
	s := newNoticeTestSession(t, []string{`"public"`})
	if err := s.handleNotice(notice("WARNING", noticeMsg)); err != nil {
		t.Errorf("schema ignore should silence the notice, got %v", err)
	}
}

func TestHandleNotice_MissingIdentity_IgnoreTable(t *testing.T) {
	const noticeMsg = `no tuple identifier for DELETE in table "public"."orders"`
	s := newNoticeTestSession(t, []string{`"public"."orders"`})
	if err := s.handleNotice(notice("WARNING", noticeMsg)); err != nil {
		t.Errorf("table ignore should silence the notice, got %v", err)
	}
}

func TestHandleNotice_MissingIdentity_DifferentTable(t *testing.T) {
	// Ignoring "public.orders" must not silence "other.orders".
	const noticeMsg = `no tuple identifier for UPDATE in table "other"."orders"`
	s := newNoticeTestSession(t, []string{`"public"."orders"`})
	if err := s.handleNotice(notice("WARNING", noticeMsg)); err == nil {
		t.Error("different schema should not be ignored")
	}
}

// Local Variables:
// tab-width: 4
// End:

func makeMap(ignMissId []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ignMissId))
	for _, el := range ignMissId {
		m[el] = struct{}{}
	}
	return m
}
