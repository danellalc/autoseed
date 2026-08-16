package autoseed_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
)

type fakeSource struct {
	entities []autoseed.Entity
	err      error
}

func (s fakeSource) Entities() ([]autoseed.Entity, error) {
	return s.entities, s.err
}

func TestExplain_Report(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{
		{Name: "Order", References: []autoseed.Reference{
			{Fields: []string{"ContactID"}, Target: "Contact", Nullable: false},
		}},
		{Name: "Contact", References: []autoseed.Reference{
			{Fields: []string{"DefaultOrderID"}, Target: "Order", Nullable: true},
		}},
	}}

	plan, err := autoseed.Explain(source, autoseed.SkipReason{
		Entity: "Comment", Field: "OwnerID", Reason: "polymorphic association",
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	want := "Insertion order:\n" +
		"  1. Contact\n" +
		"  2. Order\n" +
		"\nDeferred (second-pass) references:\n" +
		"  Contact.DefaultOrderID -> Order\n" +
		"\nSkipped:\n" +
		"  Comment.OwnerID: polymorphic association\n"

	if got := plan.Report(); got != want {
		t.Fatalf("Report() =\n%s\nwant\n%s", got, want)
	}
}

func TestExplain_ReportOmitsEmptySections(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{{Name: "Customer"}}}

	plan, err := autoseed.Explain(source)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	want := "Insertion order:\n  1. Customer\n"
	if got := plan.Report(); got != want {
		t.Fatalf("Report() = %q, want %q", got, want)
	}
}

func TestExplain_SkippedSortedRegardlessOfInputOrder(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{{Name: "Video"}, {Name: "Comment"}}}

	plan, err := autoseed.Explain(source,
		autoseed.SkipReason{Entity: "Video", Field: "OwnerID", Reason: "polymorphic association"},
		autoseed.SkipReason{Entity: "Comment", Field: "OwnerID", Reason: "polymorphic association"},
	)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if len(plan.Skipped) != 2 || plan.Skipped[0].Entity != "Comment" || plan.Skipped[1].Entity != "Video" {
		t.Fatalf("Skipped = %v, want Comment before Video", plan.Skipped)
	}
}

func TestExplain_PropagatesModelSourceError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := autoseed.Explain(fakeSource{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want wrapped %v", err, wantErr)
	}
}

func TestExplain_PropagatesCycleError(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{
		{Name: "Employee", References: []autoseed.Reference{
			{Fields: []string{"ManagerID"}, Target: "Employee", Nullable: false},
		}},
	}}

	_, err := autoseed.Explain(source)
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
}

func TestExplain_PropagatesUnknownReferenceError(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{
		{Name: "Order", References: []autoseed.Reference{
			{Fields: []string{"CustomerID"}, Target: "Customer"},
		}},
	}}

	_, err := autoseed.Explain(source)
	if !errors.Is(err, autoseed.ErrUnknownReference) {
		t.Fatalf("got %v, want ErrUnknownReference", err)
	}
}

func TestExplain_ReportJoinsCompositeFields(t *testing.T) {
	source := fakeSource{entities: []autoseed.Entity{
		{Name: "OrderLine"},
		{Name: "Shipment", References: []autoseed.Reference{
			{Fields: []string{"LineOrderID", "LineLineNo"}, Target: "OrderLine", Nullable: true},
		}},
	}}

	// Force a cycle so the composite reference is deferred and shows up in
	// the report with its fields joined by "+".
	source.entities[0].References = []autoseed.Reference{
		{Fields: []string{"ShipmentID"}, Target: "Shipment", Nullable: false},
	}

	plan, err := autoseed.Explain(source)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !strings.Contains(plan.Report(), "Shipment.LineOrderID+LineLineNo -> OrderLine") {
		t.Fatalf("Report() = %q, want it to contain the joined composite field reference", plan.Report())
	}
}
