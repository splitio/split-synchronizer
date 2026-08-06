package persistent

// ChangesItem represents an changesItem service response
type ChangesItem struct {
	ChangeNumber int64  `json:"changeNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	JSON         string
}

// ChangesItem Sortable list
type ChangesItems struct {
	items []ChangesItem
}

func NewChangesItems(size int) ChangesItems {
	return ChangesItems{
		items: make([]ChangesItem, 0, size),
	}
}

func (c *ChangesItems) Append(item ChangesItem) {
	c.items = append(c.items, item)
}

func (c *ChangesItems) Len() int {
	return len(c.items)
}

func (c *ChangesItems) Less(i, j int) bool {
	return c.items[i].ChangeNumber > c.items[j].ChangeNumber
}

func (c *ChangesItems) Swap(i, j int) {
	c.items[i], c.items[j] = c.items[j], c.items[i]
}

//----------------------------------------------------
