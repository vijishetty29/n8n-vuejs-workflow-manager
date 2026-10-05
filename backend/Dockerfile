FROM golang:1.22-alpine
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go mod download
RUN go build -o server .
EXPOSE 8081
CMD ["/app/server"]
