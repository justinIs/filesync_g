package track

import (
	"fmt"
	"os"
)

type OpKind int

const (
	OpKindUpdate OpKind = iota
	OpKindRefresh
	OpKindDelete
)

type SyncOutcome struct {
	Info ManifestFileInfo
	Op   OpKind
}

type Committer struct {
	m       *Manifest
	ch      chan SyncOutcome
	done    chan struct{}
	saveErr error
}

func NewCommitter(m *Manifest) *Committer {
	c := &Committer{
		m:    m,
		ch:   make(chan SyncOutcome, 500),
		done: make(chan struct{}),
	}

	go c.run()

	return c
}

func (c *Committer) Send(oc SyncOutcome) {
	c.ch <- oc
}

func (c *Committer) Close() error {
	close(c.ch)
	<-c.done
	return c.saveErr
}

func (c *Committer) run() {
	for first := range c.ch {
		batch := []SyncOutcome{first}
	drain:
		for {
			select {
			case oc, ok := <-c.ch:
				if !ok {
					break drain
				}
				batch = append(batch, oc)
			default:
				break drain
			}
		}
		c.m.applySyncOutcomes(batch...)
		if err := c.m.save(); err != nil {
			c.saveErr = err
			fmt.Fprintf(os.Stderr, "committer: save failed: %v\n", err)
		}
	}
	close(c.done)
}
