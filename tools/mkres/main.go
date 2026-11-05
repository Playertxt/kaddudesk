// mkres writes a COFF .syso containing an application manifest and icon, which the Go
// linker embeds into the .exe automatically. Usage:
//
//	go run ./tools/mkres -ico icon.ico -manifest app.manifest -o rsrc_windows_amd64.syso
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"os"
	"sort"
)

type res struct {
	typ, id uint32
	data    []byte
}

func main() {
	ico := flag.String("ico", "", "icon .ico file")
	man := flag.String("manifest", "", "manifest xml")
	out := flag.String("o", "rsrc_windows_amd64.syso", "output .syso")
	flag.Parse()

	var rs []res
	if *ico != "" {
		b, err := os.ReadFile(*ico)
		if err != nil {
			log.Fatal(err)
		}
		n := int(binary.LittleEndian.Uint16(b[4:]))
		grp := &bytes.Buffer{}
		binary.Write(grp, binary.LittleEndian, [3]uint16{0, 1, uint16(n)})
		for i := 0; i < n; i++ {
			e := b[6+16*i : 6+16*(i+1)]
			size := binary.LittleEndian.Uint32(e[8:])
			off := binary.LittleEndian.Uint32(e[12:])
			rs = append(rs, res{3, uint32(i + 1), b[off : off+size]})
			grp.Write(e[:12]) // width,height,colors,reserved,planes,bitcount,bytesInRes
			binary.Write(grp, binary.LittleEndian, uint16(i+1))
		}
		rs = append(rs, res{14, 1, grp.Bytes()})
	}
	if *man != "" {
		b, err := os.ReadFile(*man)
		if err != nil {
			log.Fatal(err)
		}
		rs = append(rs, res{24, 1, b})
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].typ != rs[j].typ {
			return rs[i].typ < rs[j].typ
		}
		return rs[i].id < rs[j].id
	})

	// group by type
	var types []uint32
	byType := map[uint32][]res{}
	for _, r := range rs {
		if _, ok := byType[r.typ]; !ok {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}

	// layout: L1 dir | L2 dirs | L3 dirs | data entries | data
	dirSize := func(n int) int { return 16 + 8*n }
	l1 := dirSize(len(types))
	l2Off := map[uint32]int{}
	pos := l1
	for _, t := range types {
		l2Off[t] = pos
		pos += dirSize(len(byType[t]))
	}
	l3Off := make([]int, len(rs))
	for i := range rs {
		l3Off[i] = pos
		pos += dirSize(1)
	}
	deOff := make([]int, len(rs))
	for i := range rs {
		deOff[i] = pos
		pos += 16
	}
	dataOff := make([]int, len(rs))
	for i, r := range rs {
		pos = (pos + 7) &^ 7
		dataOff[i] = pos
		pos += len(r.data)
	}
	sec := make([]byte, (pos+7)&^7)
	le := binary.LittleEndian
	putDir := func(off, n int) { le.PutUint16(sec[off+14:], uint16(n)) }
	putEnt := func(off int, id uint32, target int, dir bool) {
		le.PutUint32(sec[off:], id)
		v := uint32(target)
		if dir {
			v |= 0x80000000
		}
		le.PutUint32(sec[off+4:], v)
	}
	putDir(0, len(types))
	idx := 0
	for ti, t := range types {
		putEnt(16+8*ti, t, l2Off[t], true)
		putDir(l2Off[t], len(byType[t]))
		for ri, r := range byType[t] {
			putEnt(l2Off[t]+16+8*ri, r.id, l3Off[idx], true)
			putDir(l3Off[idx], 1)
			putEnt(l3Off[idx]+16, 0x0409, deOff[idx], false)
			le.PutUint32(sec[deOff[idx]:], uint32(dataOff[idx])) // RVA, relocated by linker
			le.PutUint32(sec[deOff[idx]+4:], uint32(len(r.data)))
			copy(sec[dataOff[idx]:], r.data)
			idx++
		}
	}

	// COFF object
	f := &bytes.Buffer{}
	nrel := len(rs)
	rawPtr := 20 + 40
	relPtr := rawPtr + len(sec)
	symPtr := relPtr + 10*nrel
	binary.Write(f, le, struct {
		Machine          uint16
		NumSections      uint16
		Time             uint32
		SymPtr           uint32
		NumSyms          uint32
		OptHdrSize, Char uint16
	}{0x8664, 1, 0, uint32(symPtr), 1, 0, 0})
	name := [8]byte{'.', 'r', 's', 'r', 'c'}
	binary.Write(f, le, struct {
		Name                    [8]byte
		VSize, VAddr, RawSize   uint32
		RawPtr, RelPtr, LinePtr uint32
		NRel, NLine             uint16
		Char                    uint32
	}{name, 0, 0, uint32(len(sec)), uint32(rawPtr), uint32(relPtr), 0, uint16(nrel), 0, 0x40000040})
	f.Write(sec)
	for i := range rs {
		binary.Write(f, le, uint32(deOff[i]))
		binary.Write(f, le, uint32(0))
		binary.Write(f, le, uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}
	// symbol: .rsrc, section 1, static
	f.Write(name[:])
	binary.Write(f, le, uint32(0))
	binary.Write(f, le, int16(1))
	binary.Write(f, le, uint16(0))
	f.WriteByte(3)
	f.WriteByte(0)
	binary.Write(f, le, uint32(4)) // empty string table
	if err := os.WriteFile(*out, f.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d resources)", *out, len(rs))
}
