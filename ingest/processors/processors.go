/*************************************************************************
 * Copyright 2018 Gravwell, Inc. All rights reserved.
 * Contact: <legal@gravwell.io>
 *
 * This software may be modified and distributed under the terms of the
 * BSD 2-clause license. See the LICENSE file for details.
 **************************************************************************/

// Package processors implements preprocessors for ingesters. The intended
// usage is to create a ProcessorSet and call ProcessorSet.Process(). Calls to
// ProcessorSet.Process() are thread-safe while Process() calls on specific
// processors is not.
package processors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gravwell/gravwell/v3/ingest/config"
	"github.com/gravwell/gravwell/v3/ingest/entry"
)

const (
	preProcSectName string = `preprocessor`
	preProcTypeName string = `type`
)

var (
	ErrUnknownProcessor = errors.New("Unknown preprocessor")
	ErrNilConfig        = errors.New("Nil configuration")
	ErrNotFound         = errors.New("Processor not found")
	ErrNotReady         = errors.New("ProcessorSet not ready")
	ErrInvalidEntry     = errors.New("ErrInvalidEntry")

	emptyStruct = []byte(`{}`)
)

type ProcessorSet struct {
	sync.Mutex
	wtr entWriter
	set []Processor
}

type ProcessorConfig map[string]*config.VariableConfig

// Processor is an interface that takes an entry and processes it, returning a new block
type Processor interface {
	Process([]*entry.Entry) ([]*entry.Entry, error) //process an data item potentially setting a tag
	Flush() []*entry.Entry
	Close() error //give the processor a chance to tidy up
}

func CheckProcessor(id string) error {
	id = strings.TrimSpace(strings.ToLower(id))
	switch id {
	case GzipProcessor:
	case JsonExtractProcessor:
	case JsonArraySplitProcessor:
	case JsonFilterProcessor:
	case RegexTimestampProcessor:
	case RegexExtractProcessor:
	case RegexRouterProcessor:
	case ForwarderProcessor:
	case VpcProcessor:
	case GravwellForwarderProcessor:
	case DropProcessor:
	case CiscoISEProcessor:
	case SrcRouterProcessor:
	case PersistentBufferProcessor:
	default:
		return ErrUnknownProcessor
	}
	return nil
}

type Tagger interface {
	NegotiateTag(name string) (entry.EntryTag, error)
	LookupTag(entry.EntryTag) (string, bool)
	KnownTags() []string
}

type entWriter interface {
	WriteEntry(*entry.Entry) error
	WriteEntryContext(context.Context, *entry.Entry) error
	WriteBatch([]*entry.Entry) error
	WriteBatchContext(context.Context, []*entry.Entry) error
}

type preprocessorBase struct {
	Type string
}

func ProcessorLoadConfig(vc *config.VariableConfig) (cfg interface{}, err error) {
	var pb preprocessorBase
	if err = vc.MapTo(&pb); err != nil {
		return
	}
	switch strings.TrimSpace(strings.ToLower(pb.Type)) {
	case DropProcessor:
		cfg, err = DropLoadConfig(vc)
	case GzipProcessor:
		cfg, err = GzipLoadConfig(vc)
	case JsonExtractProcessor:
		cfg, err = JsonExtractLoadConfig(vc)
	case JsonArraySplitProcessor:
		cfg, err = JsonArraySplitLoadConfig(vc)
	case JsonFilterProcessor:
		cfg, err = JsonFilterLoadConfig(vc)
	case RegexTimestampProcessor:
		cfg, err = RegexTimestampLoadConfig(vc)
	case RegexExtractProcessor:
		cfg, err = RegexExtractLoadConfig(vc)
	case RegexRouterProcessor:
		cfg, err = RegexRouteLoadConfig(vc)
	case ForwarderProcessor:
		cfg, err = ForwarderLoadConfig(vc)
	case VpcProcessor:
		cfg, err = VpcLoadConfig(vc)
	case GravwellForwarderProcessor:
		cfg, err = GravwellForwarderLoadConfig(vc)
	case CiscoISEProcessor:
		cfg, err = CiscoISELoadConfig(vc)
	case SrcRouterProcessor:
		cfg, err = SrcRouteLoadConfig(vc)
	case PersistentBufferProcessor:
		cfg, err = PersistentBufferLoadConfig(vc)
	default:
		err = ErrUnknownProcessor
	}
	return
}

func (pc ProcessorConfig) CheckConfig(name string) (err error) {
	if vc, ok := pc[name]; !ok || vc == nil {
		err = ErrNotFound
	} else {
		_, err = ProcessorLoadConfig(vc)
	}
	return
}

func (pc ProcessorConfig) MarshalJSON() ([]byte, error) {
	if len(pc) == 0 {
		return emptyStruct, nil
	}
	mp := map[string]interface{}{}
	for k, v := range pc {
		cfg, err := ProcessorLoadConfig(v)
		if err != nil {
			return nil, err
		} else if cfg == nil {
			continue
		}
		mp[k] = cfg
	}
	return json.Marshal(mp)
}

func (pc ProcessorConfig) getProcessor(name string, tgr Tagger) (p Processor, err error) {
	if vc, ok := pc[name]; !ok || vc == nil {
		err = ErrNotFound
	} else {
		p, err = newProcessor(vc, tgr)
	}
	return
}

func newProcessor(vc *config.VariableConfig, tgr Tagger) (p Processor, err error) {
	var pb preprocessorBase
	if err = vc.MapTo(&pb); err != nil {
		return
	}
	id := strings.TrimSpace(strings.ToLower(pb.Type))
	switch id {
	case DropProcessor:
		var cfg DropConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewDrop(cfg)
	case GzipProcessor:
		var cfg GzipDecompressorConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewGzipDecompressor(cfg)
	case JsonExtractProcessor:
		var cfg JsonExtractConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewJsonExtractor(cfg)
	case JsonArraySplitProcessor:
		var cfg JsonArraySplitConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewJsonArraySplitter(cfg)
	case JsonFilterProcessor:
		var cfg JsonFilterConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewJsonFilter(cfg)
	case RegexTimestampProcessor:
		var cfg RegexTimestampConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewRegexTimestampProcessor(cfg)
	case RegexExtractProcessor:
		var cfg RegexExtractConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewRegexExtractor(cfg)
	case RegexRouterProcessor:
		var cfg RegexRouteConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewRegexRouter(cfg, tgr)
	case ForwarderProcessor:
		var cfg ForwarderConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewForwarder(cfg, tgr)
	case VpcProcessor:
		var cfg VpcConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewVpcProcessor(cfg)
	case GravwellForwarderProcessor:
		var cfg GravwellForwarderConfig
		if cfg, err = GravwellForwarderLoadConfig(vc); err != nil {
			return
		}
		p, err = NewGravwellForwarder(cfg, tgr)
	case CiscoISEProcessor:
		var cfg CiscoISEConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewCiscoISEProcessor(cfg)
	case SrcRouterProcessor:
		var cfg SrcRouteConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewSrcRouter(cfg, tgr)
	case PersistentBufferProcessor:
		var cfg PersistentBufferConfig
		if err = vc.MapTo(&cfg); err != nil {
			return
		}
		p, err = NewPersistentBuffer(cfg)
	default:
		err = ErrUnknownProcessor
	}
	return
}

func NewProcessorSet(wtr entWriter) *ProcessorSet {
	return &ProcessorSet{
		wtr: wtr,
	}
}

func (pr *ProcessorSet) Enabled() bool {
	pr.Lock()
	defer pr.Unlock()
	return len(pr.set) > 0 && pr.wtr != nil
}

func (pr *ProcessorSet) AddProcessor(p Processor) {
	pr.Lock()
	defer pr.Unlock()
	pr.set = append(pr.set, p)
}

func (pr *ProcessorSet) Process(ent *entry.Entry) (err error) {
	if ent == nil {
		return ErrInvalidEntry
	}
	pr.Lock()
	if pr == nil || pr.wtr == nil {
		err = ErrNotReady
	} else if len(pr.set) == 0 {
		err = pr.wtr.WriteEntry(ent)
	} else {
		//we have processors, start recursing into them
		err = pr.processItems([]*entry.Entry{ent}, 0)
	}
	pr.Unlock()
	return
}

func (pr *ProcessorSet) ProcessBatch(ents []*entry.Entry) (err error) {
	if len(ents) == 0 {
		return nil
	}
	pr.Lock()
	if pr == nil || pr.wtr == nil {
		err = ErrNotReady
	} else if len(pr.set) == 0 {
		err = pr.wtr.WriteBatch(ents)
	} else {
		//we have processors, start recursing into them
		err = pr.processItems(ents, 0)
	}
	pr.Unlock()
	return
}

func (pr *ProcessorSet) ProcessContext(ent *entry.Entry, ctx context.Context) (err error) {
	if ent == nil {
		return ErrInvalidEntry
	}
	pr.Lock()
	if pr == nil || pr.wtr == nil {
		err = ErrNotReady
	} else if len(pr.set) == 0 {
		err = pr.wtr.WriteEntryContext(ctx, ent)
	} else {
		//we have processors, start recursing into them
		err = pr.processItemsContext([]*entry.Entry{ent}, 0, ctx)
	}
	pr.Unlock()
	return
}

func (pr *ProcessorSet) ProcessBatchContext(ents []*entry.Entry, ctx context.Context) (err error) {
	if len(ents) == 0 {
		return nil
	}
	pr.Lock()
	if pr == nil || pr.wtr == nil {
		err = ErrNotReady
	} else if len(pr.set) == 0 {
		err = pr.wtr.WriteBatchContext(ctx, ents)
	} else {
		//we have processors, start recursing into them
		err = pr.processItemsContext(ents, 0, ctx)
	}
	pr.Unlock()
	return
}

// processItem recurses into each processor generating entries and writing them out
func (pr *ProcessorSet) processItems(ents []*entry.Entry, i int) error {
	if i >= len(pr.set) {
		//we are at the end of the line, just write the entry
		return pr.wtr.WriteBatch(ents)
	}
	if set, err := pr.set[i].Process(ents); err != nil {
		return err
	} else {
		if err := pr.processItems(set, i+1); err != nil {
			return err
		}
	}
	return nil
}

// processItemContext recurses into each processor generating entries and writing them out
func (pr *ProcessorSet) processItemsContext(ents []*entry.Entry, i int, ctx context.Context) error {
	if i >= len(pr.set) {
		//we are at the end of the line, just write the entry
		return pr.wtr.WriteBatchContext(ctx, ents)
	}
	if set, err := pr.set[i].Process(ents); err != nil {
		return err
	} else {
		if err := pr.processItemsContext(set, i+1, ctx); err != nil {
			return err
		}
	}
	return nil
}

// Close will close the underlying preprocessors within the set.
// This function DOES NOT close the ingest muxer handle.
// It is ONLY for shutting down preprocessors
func (pr *ProcessorSet) Close() (err error) {
	for i, v := range pr.set {
		if v != nil {
			if ents := v.Flush(); len(ents) > 0 {
				if lerr := pr.processItems(ents, i+1); lerr != nil {
					err = addError(lerr, err)
				}
			}
			if lerr := v.Close(); lerr != nil {
				err = addError(lerr, err)
			}
		}
	}
	return
}

func addError(nerr, err error) error {
	if nerr == nil {
		return err
	} else if err == nil {
		return nerr
	}
	return fmt.Errorf("%v : %v", nerr, err)
}

type tagWriter interface {
	entWriter
	Tagger
}

func (pc ProcessorConfig) ProcessorSet(t tagWriter, names []string) (pr *ProcessorSet, err error) {
	if pc == nil {
		pr = NewProcessorSet(t) //nothing defined
		return
	}
	pr = NewProcessorSet(t)
	var p Processor
	for _, n := range names {
		if p, err = pc.getProcessor(n, t); err != nil {
			err = fmt.Errorf("%s %v", n, err)
			return
		}
		pr.AddProcessor(p)
	}
	return
}

func (pc ProcessorConfig) Validate() (err error) {
	for k, v := range pc {
		if _, err = ProcessorLoadConfig(v); err != nil {
			err = fmt.Errorf("Preprocessor %s config invalid: %v", k, err)
			return
		}
	}
	return
}

func (pc ProcessorConfig) CheckProcessors(set []string) (err error) {
	for _, v := range set {
		if _, ok := pc[v]; !ok {
			err = fmt.Errorf("Preprocessor %v not defined", v)
			break
		}
	}
	return
}

type nocloser struct{}

func (n nocloser) Close() error {
	return nil
}

func (n nocloser) Flush() []*entry.Entry {
	return nil
}

const (
	defaultSetAllocSize   int = 1024
	defaultSetReallocSize int = 16
)

var (
	sa, _ = NewSetAllocator(defaultSetAllocSize, defaultSetReallocSize)
)

type SetAllocator struct {
	sync.Mutex
	set         []*entry.Entry
	allocSize   int
	reallocSize int
}

func NewSetAllocator(allocSize, reallocSize int) (sa *SetAllocator, err error) {
	if allocSize <= 0 {
		allocSize = defaultSetAllocSize
	}
	if reallocSize <= 0 {
		reallocSize = defaultSetReallocSize
	}
	if reallocSize >= allocSize {
		err = errors.New("invalid alloc to realloc size")
		return
	}
	sa = &SetAllocator{
		set:         make([]*entry.Entry, allocSize),
		allocSize:   allocSize,
		reallocSize: reallocSize,
	}
	return
}

func (sa *SetAllocator) Get(cnt int) (r []*entry.Entry) {
	sa.Lock()
	if cnt > sa.reallocSize {
		r = make([]*entry.Entry, cnt)
	} else {
		if len(sa.set) < cnt {
			//reallocate
			sa.set = make([]*entry.Entry, sa.allocSize)
		}
		r = sa.set[0:cnt]
		if sa.set = sa.set[cnt:]; len(sa.set) == 0 {
			sa.set = nil //help out the GC
		}
	}
	sa.Unlock()
	return
}

func PopSet(cnt int) []*entry.Entry {
	return sa.Get(cnt)
}
