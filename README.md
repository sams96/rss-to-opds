# RSS to OPDS

A webserver that converts RSS/atom/json feeds into OPDS, and the content from
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
http://localhost:8080/https%3A%2F%2Fwheresyoured.at%2Ffeed
```

## TODO

- [ ] Download images, and include into the epub outpub
- [ ] Ability to download content from URL, for feeds that don't include the
full content
- [x] Parse sections from content, split into sections and subsections in the 
epub (done for sections)
- [ ] Handle errors
- [ ] Better html -> xhtml conversion
- [ ] Cover images
- [ ] ???
