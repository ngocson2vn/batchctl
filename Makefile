all:
	rm -f bin/batchctl
	GOOS="linux" go build -o bin/batchctl scripts/batchctl.go

release:
	github-release delete \
  		--owner ngocson2vn \
  		--token ${GITHUB_RELEASE_TOKEN} \
  		--repo batchctl \
  		--tag "v0.1.0" \
  		--name "v0.1.0" \
		batchctl
	github-release upload \
		--owner ngocson2vn \
		--token ${GITHUB_RELEASE_TOKEN} \
		--repo batchctl \
		--tag "v0.1.0" \
		--name "v0.1.0" \
		--body "Stable" bin/batchctl
