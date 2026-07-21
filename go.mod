module github.com/sams96/rss-to-opds

go 1.26.4

require (
	github.com/antchfx/xmlquery v1.5.1
	github.com/davidbyttow/govips/v2 v2.18.0
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.57.0
)

require (
	github.com/antchfx/xpath v1.3.6 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/stretchr/testify v1.8.1 // indirect
	golang.org/x/image v0.38.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/davidbyttow/govips/v2 v2.18.0 => github.com/antst/govips/v2 v2.0.0-20260612014807-dbcc405708ea
