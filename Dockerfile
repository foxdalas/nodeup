FROM golang:onbuild

RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN go build .

CMD ["go-wrapper", "run"]