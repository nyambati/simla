# Simla

Simla is a lightweight, extensible, and open-source serverless framework that enables developers to build, test, and deploy serverless applications locally. It provides a local environment that simulates the behavior of a cloud-based serverless platform, allowing developers to iterate quickly and debug their applications with ease.

## Features

-   **Local Development:** Build and test your serverless applications locally without needing to deploy them to the cloud.
-   **Extensible:** Simla's modular design allows you to extend its functionality by adding new services and runtimes.
-   **Language Agnostic:** Simla supports a variety of programming languages, so you can write your serverless functions in the language of your choice.
-   **API Gateway:** Simla includes a built-in API gateway that allows you to expose your serverless functions as HTTP endpoints.
-   **Docker Integration:** Simla uses Docker to containerize your serverless functions, ensuring that they run consistently across different environments.

## Getting Started

To get started with Simla, you'll need to have the following installed:

-   [Go](https://golang.org/doc/install) (version 1.23 or later)
-   [Docker](https://docs.docker.com/get-docker/)

Once you have these prerequisites installed, you can clone the Simla repository and build the `simla` CLI:

```bash
git clone https://github.com/nyambati/simla.git
cd simla
make build
```

This will create a `simla` binary in the `bin` directory. You can add this directory to your `PATH` to make it easier to run the `simla` command.

## Usage

To start the Simla server, run the following command:

```bash
simla up
```

This will start the Simla server and the API gateway. You can then use the API gateway to invoke your serverless functions.

To define a serverless function, you'll need to create a `.simla.yaml` file in the root of your project. This file defines the services and functions that make up your application.

Here's an example of a `.simla.yaml` file:

```yaml
apiGateway:
  scheme: http
  port: 8080
  routes:
    - path: payments
      method: GET
      service: payments
Services:
  payments:
    architecture: amd64
    environment:
      TEST_ENV: test
    codePath: ./bin
    runtime: go
    cmd: ["main"]
```

This configuration defines a service called `my-service` with a single function. The function's handler is `main.handler`, and it uses the `go1.x` runtime. The function is packaged as a Docker image called `my-service-image`.

## Architecture

Simla's architecture is composed of the following components:

-   **CLI:** The `simla` CLI is the primary interface for interacting with Simla. It provides commands for starting and stopping the Simla server, deploying services, and invoking functions.
-   **API Gateway:** The API gateway is responsible for handling incoming requests and routing them to the appropriate services.
-   **Service Registry:** The service registry keeps track of all the services and functions that are running in the Simla environment.
-   **Scheduler:** The scheduler is responsible for scheduling the execution of serverless functions.
-   **Runtime:** The runtime is responsible for executing the code of a serverless function. Simla supports a variety of runtimes, including Go, Python, and Node.js.
-   **Docker:** Simla uses Docker to containerize serverless functions. This ensures that the functions run in a consistent and isolated environment.

## Contributing

Simla is an open-source project, and we welcome contributions from the community. If you'd like to contribute, please fork the repository and submit a pull request.

## License

Simla is licensed under the [MIT License](LICENSE).
