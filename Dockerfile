FROM golang:1.19 as builder

WORKDIR /src/
COPY *.go go.mod go.sum ./
COPY kafkautils ./kafkautils
COPY message ./message
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o ia2 ./

# Our final image is based on amazoncorretto because it contains OpenJDK.  We
# need the tool keytool which is part of OpenJDK.
FROM amazoncorretto:8-alpine-jre

RUN apk add --no-cache bash

COPY --from=builder /src/ia2 /bin/
COPY start.sh /bin/
RUN chmod 755 /bin/start.sh
EXPOSE 8080

# Switch to the UID that's typically reserved for the user "nobody".
USER 65534

CMD ["/bin/start.sh"]
