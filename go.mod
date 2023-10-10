module github.com/omec-project/sctplb

go 1.21

replace github.com/omec-project/sctplb => ./

require (
	git.cs.nctu.edu.tw/calee/sctp v1.1.0
	github.com/antonfisher/nested-logrus-formatter v1.3.1
	github.com/omec-project/ngap v1.1.0
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.48.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/omec-project/aper v1.1.0 // indirect
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2 // indirect
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20200825200019-8632dd797987 // indirect
)
