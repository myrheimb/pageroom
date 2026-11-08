package merge

import "testing"

const base = "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n"

func TestDifferentParagraphsMergeCleanly(t *testing.T) {
	ours := "# Title\n\nFirst paragraph, edited by me.\n\nSecond paragraph.\n\nThird paragraph.\n"
	theirs := "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n\nThird paragraph, edited by them.\n"
	r := Merge(base, ours, theirs)
	if r.HasConflicts() {
		t.Fatalf("unexpected conflict: %+v", r.Hunks)
	}
	want := "# Title\n\nFirst paragraph, edited by me.\n\nSecond paragraph.\n\nThird paragraph, edited by them.\n"
	if got := r.Text(nil); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestSameParagraphConflicts(t *testing.T) {
	ours := "# Title\n\nFirst paragraph.\n\nMine.\n\nThird paragraph.\n"
	theirs := "# Title\n\nFirst paragraph.\n\nTheirs.\n\nThird paragraph.\n"
	r := Merge(base, ours, theirs)
	if !r.HasConflicts() {
		t.Fatal("expected conflict")
	}
	if got := r.Text([]Resolution{KeepTheirs}); got != theirs {
		t.Errorf("theirs: %q", got)
	}
	if got := r.Text([]Resolution{KeepOurs}); got != ours {
		t.Errorf("ours: %q", got)
	}
	both := "# Title\n\nFirst paragraph.\n\nMine.\n\nTheirs.\n\nThird paragraph.\n"
	if got := r.Text([]Resolution{KeepBoth}); got != both {
		t.Errorf("both: %q", got)
	}
}

func TestIdenticalChangesDoNotConflict(t *testing.T) {
	same := "# Title\n\nNew text.\n"
	r := Merge(base, same, same)
	if r.HasConflicts() || r.Text(nil) != same {
		t.Fatalf("got %+v", r)
	}
}

func TestInsertionsAndDeletions(t *testing.T) {
	ours := "# Title\n\nIntro added.\n\nFirst paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n"
	theirs := "# Title\n\nFirst paragraph.\n\nThird paragraph.\n"
	r := Merge(base, ours, theirs)
	if r.HasConflicts() {
		t.Fatalf("unexpected conflict %+v", r.Hunks)
	}
	want := "# Title\n\nIntro added.\n\nFirst paragraph.\n\nThird paragraph.\n"
	if got := r.Text(nil); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestEmptyBase(t *testing.T) {
	r := Merge("", "a\n", "b\n")
	if !r.HasConflicts() {
		t.Fatal("two different new files should conflict")
	}
	r = Merge("", "", "b\n")
	if r.HasConflicts() || r.Text(nil) != "b\n" {
		t.Fatalf("got %q", r.Text(nil))
	}
}
