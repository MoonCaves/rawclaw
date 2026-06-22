package render

import (
	"bytes"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/view"
)

func TestFmtMsg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  view.ViewMsg
		want string
	}{
		{
			name: "non-anchor uses leading space for star",
			msg:  view.ViewMsg{ID: 12, Role: "user", Text: "hello", Anchor: false},
			want: "       [user #12] hello",
		},
		{
			name: "anchor uses triangle marker",
			msg:  view.ViewMsg{ID: 3, Role: "assistant", Text: "done", Anchor: true},
			want: "     ▶ [assistant #3] done",
		},
		{
			name: "empty text still renders the brackets and id",
			msg:  view.ViewMsg{ID: 0, Role: "user", Text: "", Anchor: false},
			want: "       [user #0] ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fmtMsg(tt.msg)
			if got != tt.want {
				t.Errorf("fmtMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSID8(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "longer than 8 truncates", in: "abcdefghij", want: "abcdefgh"},
		{name: "exactly 8 unchanged", in: "abcdefgh", want: "abcdefgh"},
		{name: "shorter than 8 not padded", in: "abc", want: "abc"},
		{name: "empty stays empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sid8(tt.in); got != tt.want {
				t.Errorf("sid8(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPrintScroll(t *testing.T) {
	t.Parallel()

	scroll := &view.ScrollResult{
		Project:   "proj",
		SessionID: "deadbeefcafe",
		Around:    42,
		View: &view.AnchoredView{
			Window: []view.ViewMsg{
				{ID: 41, Role: "user", Text: "before", Anchor: false},
				{ID: 42, Role: "assistant", Text: "anchor", Anchor: true},
			},
			MessagesBefore: 1,
			MessagesAfter:  0,
		},
	}

	scrollWant := "━━ deadbeef · proj · around #42 (1 before / 0 after) ━━\n" +
		"       [user #41] before\n" +
		"     ▶ [assistant #42] anchor\n"

	notFoundWant := "Nothing to scroll (session or message id not found).\n"

	tests := []struct {
		name string
		in   *view.ScrollResult
		want string
	}{
		{name: "nil scroll", in: nil, want: notFoundWant},
		{name: "nil view", in: &view.ScrollResult{SessionID: "x", View: nil}, want: notFoundWant},
		{name: "full window", in: scroll, want: scrollWant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			PrintScroll(&buf, tt.in)
			if got := buf.String(); got != tt.want {
				t.Errorf("PrintScroll() output mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestPrintBrowse(t *testing.T) {
	t.Parallel()

	rows := []view.BrowseRow{
		{SessionID: "11112222aaaa", LastTS: 1700000000, N: 12, Preview: "first session"},
		{SessionID: "33334444", LastTS: 1700000100, N: 3, Preview: "second"},
	}

	rowsWant := "2 most-recent sessions on alpha:\n\n" +
		"  · 11112222 · 12 msgs · first session\n" +
		"  · 33334444 · 3 msgs · second\n"

	emptyWant := "No sessions on alpha.\n"

	tests := []struct {
		name    string
		rows    []view.BrowseRow
		project string
		want    string
	}{
		{name: "empty rows", rows: nil, project: "alpha", want: emptyWant},
		{name: "two rows", rows: rows, project: "alpha", want: rowsWant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			PrintBrowse(&buf, tt.rows, tt.project)
			if got := buf.String(); got != tt.want {
				t.Errorf("PrintBrowse() output mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}
