FROM golang:1.7
ADD . /go/src/github.com/pedro-r-marques/pipeline
RUN go install github.com/pedro-r-marques/pipeline/cmd/...
RUN rm -rf /go/src
ADD cmd/pipeman/static /var/www/pipeline/static
ENTRYPOINT /go/bin/pipeman
EXPOSE 8080