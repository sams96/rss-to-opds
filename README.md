# RSS to OPDS

A webserver that converts RSS/atom feeds into OPDS, and the content from
the input feed into per entry epubs. Very much a work in progress.

## Usage

run with

```bash
go run main.go
```

then add the following to your OPDS reader

```
http://localhost:8080/{url}
```

use a url encoded url, for example
```
http://localhost:8080/https%3A%2F%2Fsamsm.ch%2Findex.xml
```

## Thanks to

- [github.com/go-shiori/go-epub](https://github.com/go-shiori/go-epub)
- [github.com/antchfx/xmlquery](https://github.com/antchfx/xmlquery)
- [github.com/davidbyttow/govips](https://github.com/davidbyttow/govips) and davidbyttow/govips#530

## TODO

- [x] Download images, and include into the epub outpub
- [ ] Configurable image processing options
- [ ] Ability to download content from URL, for feeds that don't include the
full content
- [x] Parse sections from content, split into sections and subsections in the 
epub (done for sections)
- [x] Handle errors
- [ ] Better html -> xhtml conversion
- [ ] Cover images
- [ ] ???
