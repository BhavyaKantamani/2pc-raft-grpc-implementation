FROM golang:1.22

WORKDIR /app

COPY . .

# Install dependencies (use tidy instead of go get ./...)
RUN go mod init q4 && go mod tidy

RUN go mod tidy

RUN go build -o node q4_node.go

CMD ["./node"]

