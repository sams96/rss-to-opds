package epub

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"

	"github.com/google/uuid"
)

// Epub implements an EPUB file.
type Epub struct {
	author string
	// cover  *epubCover
	// The key is the css filename, the value is the css source
	// css map[string]string
	// The key is the font filename, the value is the font source
	// fonts      map[string]string
	identifier string
	// Language
	lang string
	// Description
	desc string
	// Page progression direction
	ppd string
	// The package file (package.opf)
	pkg      *pkg
	sections []*epubSection
	title    string
	// Table of contents
	toc *toc
	dst *zip.Writer
}

type epubSection struct {
	filename string
	xhtml    *xhtml
	children []*epubSection
}

const (
	defaultEpubLang = "en"
	urnUUIDPrefix   = "urn:uuid:"
)

// New starts writing a new epub
func New(title string, w io.Writer) (*Epub, error) {
	var err error
	e := &Epub{}
	e.pkg, err = newPackage()
	if err != nil {
		return nil, fmt.Errorf("can't create NewEpub: %w", err)
	}
	e.toc, err = newToc()
	if err != nil {
		return nil, fmt.Errorf("can't create NewEpub: %w", err)
	}
	// Set minimal required attributes
	e.SetIdentifier(urnUUIDPrefix + uuid.NewString())
	e.SetLang(defaultEpubLang)
	e.SetTitle(title)

	e.dst = zip.NewWriter(w)

	err = writeMimetype(e.dst)
	if err != nil {
		return nil, err
	}

	err = writeContainerFile(e.dst)
	if err != nil {
		return nil, err
	}

	return e, nil
}

// SetIdentifier sets the unique identifier of the EPUB, such as a UUID, DOI,
// ISBN or ISSN. If no identifier is set, a UUID will be automatically
// generated.
func (e *Epub) SetIdentifier(identifier string) {
	e.identifier = identifier
	e.pkg.setIdentifier(identifier)
	e.toc.setIdentifier(identifier)
}

// SetLang sets the language of the EPUB.
func (e *Epub) SetLang(lang string) {
	e.lang = lang
	e.pkg.setLang(lang)
}

// SetDescription sets the description of the EPUB.
func (e *Epub) SetDescription(desc string) {
	e.desc = desc
	e.pkg.setDescription(desc)
}

// SetPpd sets the page progression direction of the EPUB.
func (e *Epub) SetPpd(direction string) {
	e.ppd = direction
	e.pkg.setPpd(direction)
}

// SetTitle sets the title of the EPUB.
func (e *Epub) SetTitle(title string) {
	e.title = title
	e.pkg.setTitle(title)
	e.toc.setTitle(title)
}

// SetAuthor sets the author of the EPUB.
func (e *Epub) SetAuthor(author string) {
	e.author = author
	e.pkg.setAuthor(author)
}

const (
	containerFilename     = "container.xml"
	containerFileTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="%s/%s" media-type="application/oebps-package+xml" />
  </rootfiles>
</container>
`
	// This seems to be the standard based on the latest EPUB spec:
	// http://www.idpf.org/epub/31/spec/epub-ocf.html
	contentFolderName    = "EPUB"
	coverImageProperties = "cover-image"
	mediaTypeEPUB        = "application/epub+zip"
	mediaTypeNCX         = "application/x-dtbncx+xml"
	mediaTypeXHTML       = "application/xhtml+xml"
	metaInfFolderName    = "META-INF"
	mimetypeFilename     = "mimetype"
	pkgFilename          = "package.opf"
)

func (e *Epub) writeTOC(w *zip.Writer) error {
	e.pkg.addToManifest(tocNavItemID, tocNavFilename, mediaTypeXHTML, tocNavItemProperties)
	e.pkg.addToManifest(tocNcxItemID, tocNcxFilename, mediaTypeNCX, "")

	return e.toc.write(w)
}

// Write the mimetype file
//
// Spec: http://www.idpf.org/epub/301/spec/epub-ocf.html#sec-zip-container-mime
func writeMimetype(w *zip.Writer) error {
	dst, err := w.Create(mimetypeFilename)
	if err != nil {
		return err
	}

	_, err = dst.Write([]byte(mediaTypeEPUB))
	return err
}

// Write the contatiner file (container.xml), which mostly just points to the
// package file (package.opf)
//
// Spec: http://www.idpf.org/epub/301/spec/epub-ocf.html#sec-container-metainf-container.xml
func writeContainerFile(w *zip.Writer) error {
	dst, err := w.Create(filepath.Join(metaInfFolderName, containerFilename))
	if err != nil {
		return err
	}
	if _, err := dst.Write(
		fmt.Appendf([]byte{},
			containerFileTemplate,
			contentFolderName,
			pkgFilename,
		),
	); err != nil {
		return fmt.Errorf("Error writing container file: %w", err)
	}
	return nil
}

func (e *Epub) Write() (int64, error) {
	err := e.writeTOC(e.dst)
	if err != nil {
		return 0, err
	}

	err = e.pkg.write(e.dst)
	if err != nil {
		return 0, err
	}

	return 0, e.dst.Close()
}

func (e *Epub) AddSection(src io.Reader, sectionTitle string) error {
	internalFilename := fmt.Sprintf("section%04d.xhtml", len(e.sections))

	x := newXhtml(src)
	x.setTitle(sectionTitle)
	x.setXmlnsEpub(xmlnsEpub)

	e.sections = append(e.sections,
		&epubSection{
			filename: internalFilename,
			xhtml:    x,
			children: nil,
		})

	// Set the title of the cover page XHTML to the title of the EPUB
	// if section.filename == e.cover.xhtmlFilename {
	// section.xhtml.setTitle(e.Title())
	// }

	sectionFilePath := filepath.Join(contentFolderName, internalFilename)
	dst, _ := e.dst.Create(sectionFilePath)
	err := x.write(dst)
	if err != nil {
		return err
	}

	// if section.filename != e.cover.xhtmlFilename {
	// e.pkg.addToSpine(section.filename)
	// }
	e.pkg.addToSpine(internalFilename)
	e.pkg.addToManifest(internalFilename, internalFilename, "application/xhtml+xml", "")
	e.toc.addSubSection("-1", 0, x.Title(), internalFilename)
	return nil
}

type TranscoderFunc func(io.Writer, io.Reader) (int64, error)

func (e *Epub) AddMedia(src io.Reader, filename, mediaType string, transcoder TranscoderFunc) error {
	dst, err := e.dst.Create(filepath.Join(contentFolderName, filename))
	if err != nil {
		return err
	}

	_, err = transcoder(dst, src)
	if err != nil {
		return err
	}

	xmlId, err := fixXMLId(filename)
	if err != nil {
		return err
	}

	e.pkg.addToManifest(xmlId, filename, mediaType, "")
	return nil
}
