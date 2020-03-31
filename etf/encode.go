package etf

import (
	"github.com/halturin/ergo/lib"
)

func Encode(term Term, b *lib.Buffer,
	linkAtomCache *AtomCache,
	writerAtomCache map[Atom]CacheItem,
	encodingAtomCache []CacheItem) error {

	// Atom cache: (if its enabled: linkAtomCache != nil)
	// 1. check an atom in writerAtomCache (map)
	// 2. if not found in writerAtomCache call linkAtomCache.Append(atom),
	// 3. if found
	//    add encodingAtomCache[i] = CacheItem, where i is just a counter
	//    within this encoding process.
	//    encode atom as CacheRef with value = i

	return nil
}
