FROM golang:onbuild

RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN go build .
RUN ls | grep -v nodeup | xargs rm -rf

ENTRYPOINT ["go-wrapper", "run"]