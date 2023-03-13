set -ex

cd cmd/rsvp-lambda
GOOS=linux GOARCH=amd64 go build -o main main.go
zip main.zip main
aws lambda update-function-code --function-name RSVPer --region us-east-2 --zip-file fileb://main.zip
