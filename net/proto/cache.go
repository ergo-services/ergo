package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

const (
	cacheKindType  byte = 1
	cacheKindAtom  byte = 2
	cacheKindError byte = 3

	cacheFrameBody = 32 * 1024
)

func (c *connection) encode(v any, buf *lib.Buffer) error {
	if c.cacheUpdate && c.registry.Revision() > c.defined.Load() {
		c.define()
	}
	return edf.Encode(v, buf, c.encodeOptions)
}

func (c *connection) define() {
	c.dictMutex.Lock()
	defer c.dictMutex.Unlock()

	known := c.defined.Load()
	d := c.registry.Since(known)
	if d.Revision == known {
		return
	}
	frames := cacheFrames(d)

	c.pool_mutex.RLock()
	pool := append([]*pool_item(nil), c.pool...)
	c.pool_mutex.RUnlock()

	for _, pi := range pool {
		writeFrames(pi, frames)
	}
	c.defined.Store(d.Revision)
}

func (c *connection) prime(pi *pool_item) {
	writeFrames(pi, cacheFrames(c.registry.Since(c.snapshot)))
}

func writeFrames(pi *pool_item, frames [][]byte) {
	for _, frame := range frames {
		if _, err := pi.fl.Write(frame); err != nil {
			pi.closeConn()
			return
		}
	}
}

func cacheFrames(d edf.Dictionary) [][]byte {
	var frames [][]byte
	var frame []byte
	add := func(kind byte, id uint16, text string) {
		if frame == nil {
			frame = make([]byte, 8, 8+cacheFrameBody)
		}
		frame = append(frame, kind, byte(id>>8), byte(id), byte(len(text)>>8), byte(len(text)))
		frame = append(frame, text...)
		if len(frame)-8 >= cacheFrameBody {
			frames = append(frames, sealCacheFrame(frame))
			frame = nil
		}
	}
	for id, name := range d.Types {
		add(cacheKindType, id, name)
	}
	for id, atom := range d.Atoms {
		add(cacheKindAtom, id, string(atom))
	}
	for id, err := range d.Errors {
		add(cacheKindError, id, err.Error())
	}
	if frame != nil {
		frames = append(frames, sealCacheFrame(frame))
	}
	return frames
}

func sealCacheFrame(frame []byte) []byte {
	frame[0] = protoMagic
	frame[1] = protoVersion
	binary.BigEndian.PutUint32(frame[2:6], uint32(len(frame)))
	frame[6] = 0
	frame[7] = protoMessageC
	return frame
}

type cacheItem struct {
	kind byte
	id   uint16
	text string
}

func (c *connection) installCache(body []byte) error {
	var items []cacheItem
	for len(body) > 0 {
		if len(body) < 5 {
			return fmt.Errorf("truncated entry")
		}
		item := cacheItem{kind: body[0], id: binary.BigEndian.Uint16(body[1:3])}
		l := int(binary.BigEndian.Uint16(body[3:5]))
		if len(body) < 5+l {
			return fmt.Errorf("truncated entry")
		}
		item.text = string(body[5 : 5+l])
		body = body[5+l:]
		if err := c.checkCacheItem(item); err != nil {
			return err
		}
		items = append(items, item)
	}

	var types, errs []string
	for _, item := range items {
		switch item.kind {
		case cacheKindType:
			c.decodeOptions.RegCache.LoadOrStore(item.id, item.text)
			if _, known := c.registry.LookupType(item.text); known == false {
				types = append(types, item.text)
			}
		case cacheKindAtom:
			c.decodeOptions.AtomCache.LoadOrStore(item.id, gen.Atom(item.text))
		case cacheKindError:
			c.decodeOptions.ErrCache.LoadOrStore(item.id, c.registry.RemoteError(errors.New(item.text)))
			if _, known := c.registry.Sentinel(item.text); known == false {
				errs = append(errs, item.text)
			}
		}
	}
	c.reportUnknown(types, errs)
	return nil
}

func (c *connection) checkCacheItem(item cacheItem) error {
	var cache *sync.Map
	var same func(v any) bool
	switch item.kind {
	case cacheKindType:
		if item.id < 4096 || item.text == "" {
			return fmt.Errorf("incorrect type entry %d", item.id)
		}
		cache = c.decodeOptions.RegCache
		same = func(v any) bool { return v.(string) == item.text }
	case cacheKindAtom:
		if item.id < 256 || len(item.text) > 255 {
			return fmt.Errorf("incorrect atom entry %d", item.id)
		}
		cache = c.decodeOptions.AtomCache
		same = func(v any) bool { return v.(gen.Atom) == gen.Atom(item.text) }
	case cacheKindError:
		if item.id <= math.MaxInt16 || item.id > math.MaxUint16-2 {
			return fmt.Errorf("incorrect error entry %d", item.id)
		}
		cache = c.decodeOptions.ErrCache
		same = func(v any) bool { return errorText(v) == item.text }
	default:
		return fmt.Errorf("unknown entry kind %d", item.kind)
	}
	if v, found := cache.Load(item.id); found && same(v) == false {
		return fmt.Errorf("cache id %d is taken by %v", item.id, v)
	}
	return nil
}

func (c *connection) reportUnknown(types, errs []string) {
	if len(types) > 0 {
		sort.Strings(types)
		c.log.Warning("%s announced %d type(s) not registered locally (yet): %s",
			c.peer, len(types), strings.Join(types, ", "))
	}
	if len(errs) > 0 {
		quoted := make([]string, len(errs))
		for i, e := range errs {
			quoted[i] = strconv.Quote(e)
		}
		sort.Strings(quoted)
		c.log.Warning("%s announced %d error(s) with no local sentinel: %s",
			c.peer, len(errs), strings.Join(quoted, ", "))
	}
}

func (c *connection) reportSnapshot() {
	var types, errs []string
	c.decodeOptions.RegCache.Range(func(_, v any) bool {
		if _, known := c.registry.LookupType(v.(string)); known == false {
			types = append(types, v.(string))
		}
		return true
	})
	c.decodeOptions.ErrCache.Range(func(_, v any) bool {
		text := errorText(v)
		if _, known := c.registry.Sentinel(text); known == false {
			errs = append(errs, text)
		}
		return true
	})
	c.reportUnknown(types, errs)
}

func errorText(v any) string {
	switch e := v.(type) {
	case edf.RemoteError:
		return e.Value().Error()
	case error:
		return e.Error()
	}
	return ""
}
