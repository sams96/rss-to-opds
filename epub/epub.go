package epub

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// Epub implements an EPUB file.
type Epub struct {
	sync.Mutex
	author string
	// cover  *epubCover
	// The key is the css filename, the value is the css source
	// css map[string]string
	// The key is the font filename, the value is the font source
	// fonts      map[string]string
	identifier string
	media      []*epubMedia
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
}

type epubSection struct {
	filename string
	xhtml    *xhtml
	children []*epubSection
}

type epubMedia struct {
	filename  string
	mediaType string
	src       io.Reader
}

const (
	defaultEpubLang = "en"
	urnUUIDPrefix   = "urn:uuid:"
)

// NewEpub returns a new Epub.
func NewEpub(title string) (*Epub, error) {
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

	return e, nil
}

// SetIdentifier sets the unique identifier of the EPUB, such as a UUID, DOI,
// ISBN or ISSN. If no identifier is set, a UUID will be automatically
// generated.
func (e *Epub) SetIdentifier(identifier string) {
	e.Lock()
	defer e.Unlock()
	e.identifier = identifier
	e.pkg.setIdentifier(identifier)
	e.toc.setIdentifier(identifier)
}

// SetLang sets the language of the EPUB.
func (e *Epub) SetLang(lang string) {
	e.Lock()
	defer e.Unlock()
	e.lang = lang
	e.pkg.setLang(lang)
}

// SetDescription sets the description of the EPUB.
func (e *Epub) SetDescription(desc string) {
	e.Lock()
	defer e.Unlock()
	e.desc = desc
	e.pkg.setDescription(desc)
}

// SetPpd sets the page progression direction of the EPUB.
func (e *Epub) SetPpd(direction string) {
	e.Lock()
	defer e.Unlock()
	e.ppd = direction
	e.pkg.setPpd(direction)
}

// SetTitle sets the title of the EPUB.
func (e *Epub) SetTitle(title string) {
	e.Lock()
	defer e.Unlock()
	e.title = title
	e.pkg.setTitle(title)
	e.toc.setTitle(title)
}

// SetAuthor sets the author of the EPUB.
func (e *Epub) SetAuthor(author string) {
	e.Lock()
	defer e.Unlock()
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
	contentFolderName    = "OPBPS"
	coverImageProperties = "cover-image"
	// Permissions for any new directories we create
	dirPermissions = 0755
	// Permissions for any new files we create
	filePermissions = 0644
	// mediaTypeCSS      = "text/css"
	mediaTypeEPUB     = "application/epub+zip"
	mediaTypeJpeg     = "image/jpeg"
	mediaTypeNCX      = "application/x-dtbncx+xml"
	mediaTypeXHTML    = "application/xhtml+xml"
	metaInfFolderName = "META-INF"
	mimetypeFilename  = "mimetype"
	pkgFilename       = "package.opf"
	tempDirPrefix     = "go-epub"
	xhtmlFolderName   = "xhtml"
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

func (e *Epub) writeSections(w *zip.Writer) error {
	for _, section := range e.sections {

		// Set the title of the cover page XHTML to the title of the EPUB
		// if section.filename == e.cover.xhtmlFilename {
		// section.xhtml.setTitle(e.Title())
		// }

		sectionFilePath := filepath.Join(contentFolderName, section.filename)
		dst, _ := w.Create(sectionFilePath)
		err := section.xhtml.write(dst)
		if err != nil {
			return err
		}

		relativePath := filepath.Join(section.filename)
		// if section.filename != e.cover.xhtmlFilename {
		// e.pkg.addToSpine(section.filename)
		// }
		e.pkg.addToSpine(section.filename)
		e.pkg.addToManifest(section.filename, relativePath, "application/xhtml+xml", "")
		e.toc.addSubSection("-1", 0, section.xhtml.Title(), relativePath)
	}

	return nil
}

func (e *Epub) writeMedia(w *zip.Writer) error {
	for _, file := range e.media {
		dst, err := w.Create(filepath.Join(contentFolderName, file.filename))
		if err != nil {
			return err
		}

		_, err = io.Copy(dst, file.src)
		if err != nil {
			return err
		}

		xmlId, err := fixXMLId(file.filename)
		if err != nil {
			return err
		}

		e.pkg.addToManifest(xmlId, file.filename, file.mediaType, "")
	}

	return nil
}

func (e *Epub) WriteTo(w io.Writer) (int64, error) {
	dst := zip.NewWriter(w)

	err := writeMimetype(dst)
	if err != nil {
		return 0, err
	}

	err = writeContainerFile(dst)
	if err != nil {
		return 0, err
	}

	err = e.writeSections(dst)
	if err != nil {
		return 0, err
	}

	err = e.writeMedia(dst)
	if err != nil {
		return 0, err
	}

	err = e.writeTOC(dst)
	if err != nil {
		return 0, err
	}

	err = e.pkg.write(dst)
	if err != nil {
		return 0, err
	}

	return 0, dst.Close()
}

func (e *Epub) AddSection(body, sectionTitle string) (string, error) {
	internalFilename := fmt.Sprintf("section%04d.xhtml", len(e.sections))

	x, err := newXhtml(body)
	if err != nil {
		return internalFilename, fmt.Errorf("can't add section we cant create xhtml: %w", err)
	}
	x.setTitle(sectionTitle)
	x.setXmlnsEpub(xmlnsEpub)

	e.sections = append(e.sections,
		&epubSection{
			filename: internalFilename,
			xhtml:    x,
			children: nil,
		})
	return internalFilename, nil
}

func (e *Epub) AddMedia(src io.Reader, filename, mediaType string) {
	e.media = append(e.media, &epubMedia{filename, mediaType, src})
}
