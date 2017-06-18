package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// IntSet is a set of integers.
type IntSet map[int]bool

// XRefTableEntry represents an entry in the PDF cross reference table.
//
// This may be a free object, a compressed object or any in use PDF object:
//
// PDFDict, PDFStreamDict, PDFObjectStreamDict, PDFXRefStreamDict,
// PDFArray, PDFInteger, PDFFloat, PDFName, PDFStringLiteral, PDFHexLiteral, PDFBoolean
type XRefTableEntry struct {
	Free            bool
	Offset          *int64
	Generation      *int
	Object          interface{}
	Compressed      bool
	ObjectStream    *int
	ObjectStreamInd *int
}

// NewXRefTableEntryGen0 creates a cross reference table entry for an object with generation 0.
func NewXRefTableEntryGen0() *XRefTableEntry {
	zero := 0
	return &XRefTableEntry{Generation: &zero}
}

// XRefTable represents a PDF cross reference table plus stats for a PDF file.
type XRefTable struct {
	Table     map[int]*XRefTableEntry
	Size      *int            // Object count from PDF trailer dict.
	PageCount int             // Number of pages.
	Root      *PDFIndirectRef // Catalog (reference to root object).

	// PDF Version
	HeaderVersion *PDFVersion // The PDF version the source is claiming to us as per its header.
	RootVersion   *PDFVersion // Optional PDF version taking precedence over the header version.

	// Document information section
	Info     *PDFIndirectRef // Infodict (reference to info dict object)
	ID       *PDFArray       // from info dict (or trailer?)
	Author   string
	Creator  string
	Producer string

	// Linearization section (not yet supported)
	OffsetPrimaryHintTable  *int64
	OffsetOverflowHintTable *int64
	LinearizationObjs       IntSet

	// Offspec section
	AdditionalStreams []PDFIndirectRef //trailer :e.g., Oasis "Open Doc"

	// Statistics
	Stats PDFStats

	Tagged bool // File is using tags. This is important for ???

	// Validation
	Valid          bool // true means successful validated against ISO 32000.
	ValidationMode int  // see Configuration

	Optimized bool
}

// NewXRefTable creates a new XRefTable.
func newXRefTable(validationMode int) (xRefTable *XRefTable) {
	return &XRefTable{
		Table: map[int]*XRefTableEntry{},
		//FreeObjects:       []int{}, // slices intialisieren oder nicht?
		LinearizationObjs: IntSet{},
		Stats:             NewPDFStats(),
		ValidationMode:    validationMode,
	}
}

// Version returns the PDF version of the PDF writer that created this file.
// Before V1.4 this is the header version.
// Since V1.4 the catalog may contain a Version entry which takes precedence over the header version.
func (xRefTable *XRefTable) Version() PDFVersion {

	// TODO relaxed, RootVersion has always priority.
	//if *ctx.Read.HeaderVersion >= V14 && *ctx.Read.RootVersion != nil {

	if xRefTable.RootVersion != nil {
		return *xRefTable.RootVersion
	}

	return *xRefTable.HeaderVersion
}

// VersionString return a string representation for this PDF files PDF version.
func (xRefTable *XRefTable) VersionString() string {
	return VersionString(xRefTable.Version())
}

// ParseRootVersion returns a string representation for an optional Version entry in the root object.
func (xRefTable *XRefTable) ParseRootVersion() (*string, error) {

	// Look in the catalog/root for a name entry "Version"
	// this should be the PDFVersion of this file.

	rootDict, err := xRefTable.Catalog()
	if err != nil {
		return nil, err
	}

	// Optional
	if _, ok := rootDict.Find("Version"); !ok {
		return nil, nil
	}

	v := rootDict.PDFNameEntry("Version")
	if v != nil {
		s := v.String()
		return &s, nil
	}

	indirectRef := rootDict.IndirectRefEntry("Version")
	if indirectRef == nil {
		return nil, errors.New("ParseRootVersion: corrupt \"Version\" in root")
	}

	pdfObject, err := xRefTable.object(indirectRef)
	if err != nil {
		return nil, err
	}

	name, ok := pdfObject.(PDFName)
	if !ok {
		return nil, errors.New("ParseRootVersion: corrupt \"Version\" in root")
	}

	s := name.String()
	return &s, nil
}

// IsLinearizationObject returns true if object #i is a a linearization object.
func (xRefTable *XRefTable) IsLinearizationObject(i int) bool {
	return xRefTable.LinearizationObjs[i]
}

// LinearizationObjsString returns a formatted string and the number of objs.
func (xRefTable *XRefTable) LinearizationObjsString() (int, string) {

	var objs []int
	for k := range xRefTable.LinearizationObjs {
		if xRefTable.LinearizationObjs[k] {
			objs = append(objs, k)
		}
	}
	sort.Ints(objs)

	var linObj []string
	for _, i := range objs {
		linObj = append(linObj, fmt.Sprintf("%d", i))
	}

	return len(linObj), strings.Join(linObj, ",")
}

// Exists returns true if xRefTable contains an entry for objNumber.
func (xRefTable *XRefTable) Exists(objNumber int) bool {
	_, found := xRefTable.Table[objNumber]
	return found
}

// Find returns the XRefTable entry for given object number.
func (xRefTable *XRefTable) Find(objNumber int) (*XRefTableEntry, bool) {
	e, found := xRefTable.Table[objNumber]
	if !found {
		return nil, false
	}
	return e, true
}

// Free returns the cross ref table entry for given number of a free object.
func (xRefTable *XRefTable) Free(objNumber int) (entry *XRefTableEntry, err error) {

	entry, found := xRefTable.Find(objNumber)

	if !found {
		err = errors.Errorf("GetFree: object #%d not found.", objNumber)
		return
	}

	if !entry.Free {
		err = errors.Errorf("GetFree: object #%d found, but not free.", objNumber)
	}

	return
}

// NextForFree returns the number of the object the free object with objNumber links to.
// This is the successor of this free object in the free list.
func (xRefTable *XRefTable) NextForFree(objNumber int) (next int, err error) {

	entry, err := xRefTable.Free(objNumber)
	if err != nil {
		return
	}

	next = int(*entry.Offset)

	return
}

// FindTableEntryLight returns the XRefTable entry for given object number.
func (xRefTable *XRefTable) FindTableEntryLight(objNumber int) (*XRefTableEntry, bool) {
	return xRefTable.Find(objNumber)
}

// FindTableEntry returns the XRefTable entry for given object and generation numbers.
func (xRefTable *XRefTable) FindTableEntry(objNumber int, generationNumber int) (*XRefTableEntry, bool) {
	entry, found := xRefTable.Find(objNumber)
	if found && *entry.Generation == generationNumber {
		return entry, found
	}
	return nil, false
}

// FindTableEntryForIndRef returns the XRefTable entry for given indirect reference.
func (xRefTable *XRefTable) FindTableEntryForIndRef(indRef *PDFIndirectRef) (*XRefTableEntry, bool) {
	if indRef == nil {
		logErrorTypes.Println("FindTableEntryForIndRef: returning false on absent indRef")
		return nil, false
	}
	return xRefTable.FindTableEntry(indRef.ObjectNumber.Value(), indRef.GenerationNumber.Value())
}

// Insert adds given xRefTableEntry at given index objNumber into the cross reference table.
// Gets called when reading in a PDF file and generating its xRefTable in memory.
func (xRefTable *XRefTable) Insert(objNumber int, xRefTableEntry XRefTableEntry) bool {
	xRefTable.Table[objNumber] = &xRefTableEntry
	return true
}

// InsertNew adds given xRefTableEntry at next new objNumber into the cross reference table.
// Only to be called once after an xRefTable has been generated completely and all trailer dicts have been processed.
// xRefTable.Size is the size entry of the first trailer dict processed.
// Called on creation of new object streams.
// Called by InsertAndUseRecycled.
func (xRefTable *XRefTable) InsertNew(xRefTableEntry XRefTableEntry) (objNumber int, ok bool) {
	objNumber = *xRefTable.Size
	ok = xRefTable.Insert(objNumber, xRefTableEntry)
	*xRefTable.Size++
	return
}

// InsertAndUseRecycled adds given xRefTableEntry into the cross reference table utilizing the freelist.
// Called on creation of new xref stream only.
func (xRefTable *XRefTable) InsertAndUseRecycled(xRefTableEntry XRefTableEntry) (objNumber int, err error) {

	// see 7.5.4 Cross-Reference Table

	logDebugTypes.Println("InsertAndUseRecycled: begin")

	// Get Next free object from freelist.
	freeListHeadEntry, err := xRefTable.Free(0)
	if err != nil {
		return
	}

	// if none available, add new object & return.
	if *freeListHeadEntry.Offset == 0 {

		i, ok := xRefTable.InsertNew(xRefTableEntry)
		if !ok {
			err = errors.Errorf("InsertAndRecycle: Problem inserting entry for %d", i)
			return
		}

		objNumber = i
		logInfoTypes.Printf("InsertAndUseRecycled: end, new objNr=%d\n", objNumber)
		return
	}

	// recycle free object, update free list & return.
	objNumber = int(*freeListHeadEntry.Offset)
	entry, found := xRefTable.FindTableEntryLight(objNumber)
	if !found {
		err = errors.Errorf("InsertAndRecycle: no entry for obj #%d\n", objNumber)
		return
	}

	freeListHeadEntry.Offset = entry.Offset
	entry.Free = false
	entry.Offset = nil
	// entry is now garbage

	if !xRefTable.Insert(objNumber, xRefTableEntry) {
		err = errors.Errorf("InsertAndRecycle: Problem inserting entry for %d", objNumber)
	}

	logInfoTypes.Printf("InsertAndUseRecycled: end, recycled objNr=%d\n", objNumber)

	return
}

// EnsureValidFreeList ensures the integrity of the free list associated with the recorded free objects.
// See 7.5.4 Cross-Reference Table
func (xRefTable *XRefTable) EnsureValidFreeList() (err error) {

	logDebugTypes.Println("EnsureValidFreeList begin")

	m := IntSet{}
	for k, v := range xRefTable.Table {
		if v.Free && k > 0 {
			m[k] = true
		}
	}

	// Verify free object 0 as free list head.
	head, err := xRefTable.Free(0)
	if err != nil {
		return err
	}

	// verify generation of 56535
	if *head.Generation != FreeHeadGeneration {
		// Fix generation for obj 0.
		*head.Generation = FreeHeadGeneration
	}

	if len(m) == 0 {

		// no free object other than 0.

		// repair if necessary
		if *head.Offset != 0 {
			*head.Offset = 0
		}

		logInfoTypes.Println("EnsureValidFreeList: empty free list.")
		return
	}

	f := int(*head.Offset)

	// until we have found the last free object which should point to obj 0.
	for f != 0 {

		logDebugTypes.Printf("EnsureValidFreeList: validating obj #%d %v\n", f, m)
		// verify if obj f is one of the free objects recorded.
		if !m[f] {
			return errors.New("EnsureValidFreeList: freelist corrupted")
		}

		delete(m, f)

		f, err = xRefTable.NextForFree(f)
		if err != nil {
			return err
		}
	}

	if len(m) == 0 {
		logInfoTypes.Println("EnsureValidFreeList: end, regular linked list")
		return
	}

	// insert remaining free objects into verified linked list
	// unless they are forever deleted with generation 65535.
	// In that case they have to point to obj 0.
	for i := range m {

		entry, found := xRefTable.FindTableEntryLight(i)
		if !found {
			return errors.Errorf("EnsureValidFreeList: no xref entry found for obj #%d\n", i)
		}

		if !entry.Free {
			return errors.Errorf("EnsureValidFreeList: xref entry is not free for obj #%d\n", i)
		}

		if *entry.Generation == FreeHeadGeneration {
			zero := int64(0)
			entry.Offset = &zero
			continue
		}

		entry.Offset = head.Offset
		next := int64(i)
		head.Offset = &next
	}

	logInfoTypes.Println("EnsureValidFreeList: end, linked list plus some dangling free objects.")

	return
}

// DeleteObject marks an object as free and inserts it into the free list right after the head.
func (xRefTable *XRefTable) DeleteObject(objectNumber int) (err error) {

	// see 7.5.4 Cross-Reference Table

	logDebugTypes.Printf("DeleteObject: begin %d\n", objectNumber)

	freeListHeadEntry, err := xRefTable.Free(0)
	if err != nil {
		return
	}

	entry, found := xRefTable.FindTableEntryLight(objectNumber)
	if !found {
		err = errors.Errorf("DeleteObject: no entry for obj #%d\n", objectNumber)
		return
	}

	*entry.Generation++
	entry.Free = true
	entry.Compressed = false
	entry.Offset = freeListHeadEntry.Offset
	entry.Object = nil
	// TODO reset other fields?

	next := int64(objectNumber)
	freeListHeadEntry.Offset = &next

	logInfoTypes.Printf("DeleteObject: end %d\n", objectNumber)

	return
}

// getObject dereferences an indirect object from the xRefTable and returns the result.
func (xRefTable *XRefTable) object(indObjRef *PDFIndirectRef) (interface{}, error) {

	logDebugTypes.Printf("getObject: begin")

	if indObjRef == nil {
		return nil, errors.New("getObject: input argument is nil")
	}

	logDebugTypes.Printf("getObject: != nil")

	objectNumber := indObjRef.ObjectNumber.Value()

	generationNumber := indObjRef.GenerationNumber.Value()

	entry, found := xRefTable.FindTableEntry(objectNumber, generationNumber)
	if !found {
		return nil, errors.Errorf("getObject(obj#%d, gen#%d): xref table entry not found", objectNumber, generationNumber)
	}

	logDebugTypes.Printf("getObject: found xRefTable entry")

	if entry.Free {
		logDebugTypes.Printf("getObject(obj#%d, gen#%d): entry is free", objectNumber, generationNumber)
		return nil, nil
	}

	if entry.Object == nil {
		logDebugTypes.Printf("getObject(obj#%d, gen#%d): entry.Object is nil", objectNumber, generationNumber)
		return nil, nil
	}

	logDebugTypes.Printf("getObject: end")

	// return dereferenced object
	return entry.Object, nil
}

// Dereference resolves an indirect object and returns the resulting PDF object.
func (xRefTable *XRefTable) Dereference(obj interface{}) (interface{}, error) {

	indRef, ok := obj.(PDFIndirectRef)
	if !ok {
		// Nothing do dereference.
		return obj, nil
	}

	return xRefTable.object(&indRef)
}

// DereferenceInteger resolves and validates an integer object, which may be an indirect reference.
func (xRefTable *XRefTable) DereferenceInteger(obj interface{}) (ip *PDFInteger, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	i, ok := obj.(PDFInteger)
	if !ok {
		err = errors.Errorf("ValidateInteger: wrong type <%v>", obj)
	}

	ip = &i

	return
}

// DereferenceName resolves and validates a name object, which may be an indirect reference.
func (xRefTable *XRefTable) DereferenceName(obj interface{}, sinceVersion PDFVersion, validate func(string) bool) (n PDFName, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	n, ok := obj.(PDFName)
	if !ok {
		err = errors.Errorf("ValidateName: wrong type <%v>", obj)
		return
	}

	// Version check
	if xRefTable.Version() < sinceVersion {
		err = errors.Errorf("ValidateName: unsupported in version %s", xRefTable.VersionString())
		return
	}

	// Validation
	if validate != nil && !validate(n.Value()) {
		err = errors.Errorf("ValidateName: invalid <%s>", n.Value())
		return
	}

	return
}

// DereferenceStringLiteral resolves and validates a string literal object, which may be an indirect reference.
func (xRefTable *XRefTable) DereferenceStringLiteral(obj interface{}, sinceVersion PDFVersion, validate func(string) bool) (s PDFStringLiteral, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	s, ok := obj.(PDFStringLiteral)
	if !ok {
		err = errors.Errorf("ValidateStringLiteral: wrong type <%v>", obj)
		return
	}

	// Ensure UTF16 correctness.
	s1, err := StringLiteralToString(s.Value())
	if err != nil {
		return
	}

	// Version check
	if xRefTable.Version() < sinceVersion {
		err = errors.Errorf("ValidateStringLiteral: unsupported in version %s", xRefTable.VersionString())
		return
	}

	// Validation
	if validate != nil && !validate(s1) {
		err = errors.Errorf("ValidateStringLiteral: invalid <%s>", s1)
		return
	}

	return
}

// DereferenceStringOrHexLiteral resolves and validates a string or hex literal object, which may be an indirect reference.
func (xRefTable *XRefTable) DereferenceStringOrHexLiteral(obj interface{}, sinceVersion PDFVersion, validate func(string) bool) (o interface{}, err error) {

	o, err = xRefTable.Dereference(obj)
	if err != nil || o == nil {
		return
	}

	var s string

	switch str := o.(type) {

	case PDFStringLiteral:
		// Ensure UTF16 correctness.
		s, err = StringLiteralToString(str.Value())
		if err != nil {
			return
		}

	case PDFHexLiteral:
		// Ensure UTF16 correctness.
		s, err = HexLiteralToString(str.Value())
		if err != nil {
			return
		}

	default:
		err = errors.Errorf("ValidateStringOrHexLiteral: wrong type <%v>", obj)
		return

	}

	// Version check
	if xRefTable.Version() < sinceVersion {
		err = errors.Errorf("ValidateStringLiteral: unsupported in version %s", xRefTable.VersionString())
		return
	}

	// Validation
	if validate != nil && !validate(s) {
		err = errors.Errorf("ValidateStringLiteral: invalid <%s>", s)
		return
	}

	return
}

// DereferenceArray resolves an indirect object that points to a PDFArray.
func (xRefTable *XRefTable) DereferenceArray(obj interface{}) (arrp *PDFArray, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	arr, ok := obj.(PDFArray)
	if !ok {
		err = errors.Errorf("DereferenceArray: wrong type <%v>", obj)
	}

	arrp = &arr

	return
}

// DereferenceDict resolves an indirect object that points to a PDFDict.
func (xRefTable *XRefTable) DereferenceDict(obj interface{}) (dictp *PDFDict, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	dict, ok := obj.(PDFDict)
	if !ok {
		err = errors.Errorf("DereferenceDict: wrong type <%v>", obj)
	}

	dictp = &dict

	return
}

// DereferenceStreamDict resolves an indirect object that points to a PDFStreamDict.
func (xRefTable *XRefTable) DereferenceStreamDict(obj interface{}) (streamDictp *PDFStreamDict, err error) {

	obj, err = xRefTable.Dereference(obj)
	if err != nil || obj == nil {
		return
	}

	streamDict, ok := obj.(PDFStreamDict)
	if !ok {
		err = errors.Errorf("DereferenceStreamDict: wrong type <%v>", obj)
	}

	streamDictp = &streamDict

	return
}

// Catalog returns a pointer to the root object / catalog.
func (xRefTable *XRefTable) Catalog() (*PDFDict, error) {

	pdfObject, err := xRefTable.object(xRefTable.Root)
	if err != nil {
		return nil, err
	}

	pdfDict, ok := pdfObject.(PDFDict)
	if !ok {
		return nil, errors.New("Catalog: corrupt root catalog")
	}

	return &pdfDict, nil
}

// CatalogHasPieceInfo returns true if the root has an entry for \"PieceInfo\".
func (xRefTable *XRefTable) CatalogHasPieceInfo() (bool, error) {

	rootDict, err := xRefTable.Catalog()
	if err != nil {
		return false, err
	}

	obj, hasPieceInfo := rootDict.Find("PieceInfo")

	return hasPieceInfo && obj != nil, nil
}

// Pages returns the Pages reference contained in the catalog.
func (xRefTable *XRefTable) Pages() (*PDFIndirectRef, error) {

	rootDict, err := xRefTable.Catalog()
	if err != nil {
		return nil, err
	}

	return rootDict.IndirectRefEntry("Pages"), nil
}

// MissingObjects returns the number of objects that were not written
// plus the corresponding comma separated string representation.
func (xRefTable *XRefTable) MissingObjects() (int, *string) {

	var missing []string

	for i := 0; i < *xRefTable.Size; i++ {
		if !xRefTable.Exists(i) {
			missing = append(missing, fmt.Sprintf("%d", i))
		}
	}

	var s *string

	if len(missing) > 0 {
		joined := strings.Join(missing, ",")
		s = &joined
	}

	return len(missing), s
}

func (xRefTable *XRefTable) list(logStr []string) []string {

	var keys []int
	for k := range xRefTable.Table {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	// Print list of XRefTable entries to logString.
	for _, k := range keys {

		entry := xRefTable.Table[k]

		var str string

		if entry.Free {
			str = fmt.Sprintf("%5d: f   next=%8d generation=%d\n", k, *entry.Offset, *entry.Generation)
		} else if entry.Compressed {
			str = fmt.Sprintf("%5d: c => obj:%d[%d] generation=%d \n%s\n", k, *entry.ObjectStream, *entry.ObjectStreamInd, *entry.Generation, entry.Object)
		} else {
			if entry.Object != nil {

				typeStr := fmt.Sprintf("%T", entry.Object)

				pdfDict, ok := entry.Object.(PDFDict)

				if ok {
					if pdfDict.Type() != nil {
						typeStr += fmt.Sprintf(" type=%s", *pdfDict.Type())
					}
					if pdfDict.Subtype() != nil {
						typeStr += fmt.Sprintf(" subType=%s", *pdfDict.Subtype())
					}
				}

				if entry.ObjectStream != nil {
					// was compressed, offset is nil.
					str = fmt.Sprintf("%5d: was compressed %d[%d] generation=%d %s \n%s\n",
						k, *entry.ObjectStream, *entry.ObjectStreamInd, *entry.Generation, typeStr, entry.Object)
				} else {
					// regular in use object with offset.
					if entry.Offset != nil {
						str = fmt.Sprintf("%5d:   offset=%8d generation=%d %s \n%s\n",
							k, *entry.Offset, *entry.Generation, typeStr, entry.Object)
					} else {
						str = fmt.Sprintf("%5d:   offset=nil generation=%d %s \n%s\n",
							k, *entry.Generation, typeStr, entry.Object)
					}

				}

				if typeStr == "types.PDFStreamDict" {
					pdfStreamDict, _ := entry.Object.(PDFStreamDict)
					str += fmt.Sprintf("stream content length = %d\n", len(pdfStreamDict.Content))
					if pdfStreamDict.IsPageContent {
						// TODO logContent that drives this output.
						str += fmt.Sprintf("content: <%s>\n", pdfStreamDict.Content)
					}
				}

				if typeStr == "types.PDFObjectStreamDict" {
					pdfObjectStreamDict, _ := entry.Object.(PDFObjectStreamDict)
					str += fmt.Sprintf("object stream count:%d size of objectarray:%d\n", pdfObjectStreamDict.ObjCount, len(pdfObjectStreamDict.ObjArray))
				}

			} else {

				str = fmt.Sprintf("%5d:   offset=%8d generation=%d nil\n", k, *entry.Offset, *entry.Generation)
			}
		}

		logStr = append(logStr, str)
	}

	return logStr
}

// Dump the free list to logStr.
// At this point the free list is assumed to be a linked list with its last node linked to the beginning.
func (xRefTable *XRefTable) freeList(logStr []string) ([]string, error) {

	logDebugTypes.Printf("freeList begin")

	head, err := xRefTable.Free(0)
	if err != nil {
		return nil, err
	}

	if *head.Offset == 0 {
		return append(logStr, "\nEmpty free list.\n"), nil
	}

	f := int(*head.Offset)

	logStr = append(logStr, "\nfree list:\n  obj  next  generation\n")
	logStr = append(logStr, fmt.Sprintf("%5d %5d %5d\n", 0, f, FreeHeadGeneration))

	for f != 0 {

		logDebugTypes.Printf("freeList validating free object %d\n", f)

		entry, err := xRefTable.Free(f)
		if err != nil {
			return nil, err
		}

		next := int(*entry.Offset)
		generation := *entry.Generation
		s := fmt.Sprintf("%5d %5d %5d\n", f, next, generation)
		logStr = append(logStr, s)
		logDebugTypes.Printf("freeList: %s", s)

		f = next
	}

	return logStr, nil
}
