# Start from a base image with Go installed
FROM golang:1.21 as builder

# Set the current working directory inside the container
WORKDIR /app

COPY ./be/go.mod /be/go.sum ./
RUN go mod download

# Copy the backend source code into the container
COPY ./be .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mishis4x .


# build dist folder for
FROM node:latest as fe-builder

WORKDIR /webapp

COPY /fe/package*.json ./

RUN npm install

COPY ./fe .

CMD ["npm", "run", "build"]


# Use a small base image to run the application
FROM alpine:latest  
WORKDIR /root/

# Copy the compiled application from the builder stage
COPY --from=builder /app/mishis4x .
COPY --from=fe-builder /webapp/dist ./dist

# Expose the port the app runs on
EXPOSE 8091

# Command to run the executable
CMD ["./mishis4x", "http", "--env", "prod"]
