default:
	./compile.sh

gofmt:
	find src/*hg* -name \*.go -exec gofmt -w {} \;

