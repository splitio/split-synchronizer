package boltdb

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"testing"
	"time"
)

type CollectionItemTest struct {
	id   uint64
	Name string
}

// SetID returns identifier
func (c *CollectionItemTest) SetID(id uint64) {
	c.id = id
}

// SetID returns identifier
func (c *CollectionItemTest) ID() uint64 {
	return c.id
}

func TestCollection(t *testing.T) {

	db, err := NewInstance(fmt.Sprintf("/tmp/testcollection_%d.db", time.Now().Unix()), nil)
	if err != nil {
		t.Error(err)
	}

	item1 := &CollectionItemTest{Name: "Item 1"}
	item2 := &CollectionItemTest{Name: "Item 2"}

	col := Collection{DB: db, Name: "TEST_COLLECTION"}

	// test Save
	iid, err1 := col.Save(item1)
	if err1 != nil {
		t.Error(err1)
	}

	// test Fetch
	item1b, errf := col.Fetch(iid)
	if errf != nil {
		t.Error(errf)
	}

	q, errq := decodeItem(item1b)
	if errq != nil {
		t.Error(errq)
	}

	if q.Name != "Item 1" {
		t.Error("Invalid data fetched")
	}

	// test SaveAs
	errs := col.SaveAs([]byte(item2.Name), item2)
	if errs != nil {
		t.Error(errs)
	}

	// test FetchBy
	item2b, errfb := col.FetchBy([]byte(item2.Name))
	if errfb != nil {
		t.Error(errfb)
	}

	i2, errq2 := decodeItem(item2b)
	if errq2 != nil {
		t.Error(errq2)
	}
	if i2.Name != "Item 2" {
		t.Error("Invalid data fetched")
	}

	// test Update
	item1.Name = "Item 1 MODIFIED"
	erru := col.Update(item1)
	if erru != nil {
		t.Error(erru)
	}

	//Test FetchAll
	list, errlst := col.FetchAll()
	if errlst != nil {
		t.Error(errlst)
	}

	if len(list) != 2 {
		t.Error("Invalid len list", len(list))
	}

}

func decodeItem(item []byte) (CollectionItemTest, error) {
	var decodeBuffer bytes.Buffer
	var q CollectionItemTest

	decodeBuffer.Write(item)
	errq := gob.NewDecoder(&decodeBuffer).Decode(&q)
	return q, errq
}
