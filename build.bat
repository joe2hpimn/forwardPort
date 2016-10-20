echo "build for linux_amd64"
set GOOS=linux
set GOARCH=amd64
go build -o forwardPort


set GOOS=windows
set GOARCH=amd64
go build -o forwardPort.exe

echo "Build Success!"

pause

