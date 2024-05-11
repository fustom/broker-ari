
all:
	protoc --go_out=arimsgs --go_opt=paths=source_relative ariston.proto
	go build

