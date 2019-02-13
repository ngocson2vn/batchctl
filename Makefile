all:
	export GOOS="linux"
	go build -o bin/batchctl scripts/batchctl.go
