package gormseed

import (
	"errors"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
)

func TestExplain_EndToEnd(t *testing.T) {
	plan, err := Explain(nil, []any{&Cart{}, &CartItem{}, &Post{}, &Author{}, &Tag{}, &Video{}, &Comment{}})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	report := plan.Report()
	if !strings.Contains(report, "Insertion order:") {
		t.Fatalf("Report() missing insertion order section:\n%s", report)
	}
	if !strings.Contains(report, "Skipped:") || !strings.Contains(report, "Comment.OwnerID") {
		t.Fatalf("Report() missing the skipped polymorphic association:\n%s", report)
	}

	cartPos := strings.Index(report, "Cart\n")
	cartItemPos := strings.Index(report, "CartItem\n")
	if cartPos == -1 || cartItemPos == -1 || cartPos > cartItemPos {
		t.Fatalf("Cart must be inserted before CartItem, got:\n%s", report)
	}
}

func TestExplain_WorksWithoutADatabaseConnection(t *testing.T) {
	if _, err := Explain(nil, []any{&Order{}}); err != nil {
		t.Fatalf("Explain(nil, ...) = %v, want nil db to work: reading the model never touches a connection", err)
	}
}

func TestExplain_PropagatesUnsatisfiableCycle(t *testing.T) {
	type Employee struct {
		ID        uint `gorm:"primaryKey"`
		ManagerID uint `gorm:"not null"`
		Manager   *Employee
	}

	_, err := Explain(nil, []any{&Employee{}})
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
}
