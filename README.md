# GoWatch [WIP]

GoWatch is a lightweight observability tool tailored for Go developers building microservices in Docker and AWS Serverless stacks. It enables fast, customizable, and extensible monitoring of local services during development, providing real-time insights into performance metrics, logs, distributed tracing, resource consumption, and API status.

## Features

- **Local Service Monitoring**: Track and visualize metrics for services running locally.
- **Performance Metrics**: Monitor key performance indicators such as response times, throughput, and error rates.
- **Log Aggregation**: Collect and display logs from multiple sources for easier debugging.
- **Distributed Tracing**: Trace requests across microservices to identify bottlenecks and dependencies.
- **Resource Usage Tracking**: Monitor CPU, memory, and other resource consumption.
- **API Status Monitoring**: Check the health and status of APIs in real-time.
- **Docker Integration**: Seamlessly monitor services running in Docker containers.
- **AWS Serverless Support**: Integrate with AWS services like CloudWatch, XRay, Lambda, and CloudFormation for hybrid observability.

## Installation

To install GoWatch, ensure you have Go 1.25.3 or later installed. Clone the repository and build the project:

```bash
git clone https://github.com/b92c/GoWatch.git
cd GoWatch
go build ./cmd/obs