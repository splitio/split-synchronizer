package persistent

import (
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

// TestRBChangesWriteFailureIsReported checks that a failed Bolt write for rule-based segments
// is returned by UpdateWithError and logged by Update.
func TestRBChangesWriteFailureIsReported(t *testing.T) {
	toAdd := []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE"}}

	t.Run("UpdateWithError returns the error", func(t *testing.T) {
		c := NewRBChangesCollection(closedDB(t), logging.NewLogger(nil))
		if err := c.UpdateWithError(toAdd, nil, 1); err == nil {
			t.Error("expected the failed write to be returned")
		}
	})
	t.Run("Update logs it", func(t *testing.T) {
		rec := &errorRecorder{}
		c := NewRBChangesCollection(closedDB(t), rec)
		c.Update(toAdd, nil, 1)
		if !rec.logged("persisting rule-based segments") {
			t.Errorf("expected a logged error, got %v", rec.errs)
		}
	})
	t.Run("a healthy write returns nil", func(t *testing.T) {
		dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = dbw.Discard() })
		c := NewRBChangesCollection(dbw, logging.NewLogger(nil))
		if err := c.UpdateWithError(toAdd, nil, 7); err != nil {
			t.Errorf("unexpected error %v", err)
		}
		if c.ChangeNumber() != 7 {
			t.Errorf("change number %d, want 7", c.ChangeNumber())
		}
	})
}

// TestRBRestoreWithoutItemsAnchorsChangeNumber checks that a rule-based segments restore with no
// items still anchors the change number.
func TestRBRestoreWithoutItemsAnchorsChangeNumber(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbw.Discard() })
	c := NewRBChangesCollection(dbw, logging.NewLogger(nil))

	if err := c.UpdateWithError(nil, nil, 42); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if c.ChangeNumber() != 42 {
		t.Errorf("change number %d, want 42", c.ChangeNumber())
	}
}

func TestRBChangesCollection(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	rbChangesCollection := NewRBChangesCollection(dbw, logger)

	rbChangesCollection.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "rb2", ChangeNumber: 1, Status: "ACTIVE"},
	}, nil, 1)

	all, err := rbChangesCollection.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "rb1" || all[1].Name != "rb2" {
		t.Error("Invalid payload in fetched changes.")
	}

	if rbChangesCollection.ChangeNumber() != 1 {
		t.Error("CN should be 1.")
	}

	rbChangesCollection.Update([]dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 2, Status: "ARCHIVED"}}, nil, 2)
	all, err = rbChangesCollection.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "rb1" || all[0].Status != "ARCHIVED" {
		t.Error("rb1 should be archived.")
	}

	if rbChangesCollection.ChangeNumber() != 2 {
		t.Error("CN should be 2.")
	}
}
