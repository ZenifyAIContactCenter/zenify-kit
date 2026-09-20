# Error envelope (NestJS)

<!-- Moved out of SKILL.md on 2026-09-20 to keep the skill under the 8192-byte budget. -->

Nest's default behavior on a thrown `HttpException` is a bare
`{ statusCode, message, error }` body. A production API generally wants a
richer, consistent envelope on every error response — request id for log
correlation, a stable machine-readable error code separate from the HTTP
status, and a timestamp — produced by one global exception filter rather than
by each controller catching its own errors:

```ts
@Catch()
export class AllExceptionsFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse<Response>();
    const request = ctx.getRequest<Request>();
    const status = exception instanceof HttpException
      ? exception.getStatus()
      : HttpStatus.INTERNAL_SERVER_ERROR;

    response.status(status).json({
      status_code: status,
      timestamp: new Date().toISOString(),
      path: request.url,
      request_id: (request as any).requestId ?? 'unknown',
      error_code: exception instanceof HttpException ? exception.name : 'InternalServerError',
      message: exception instanceof HttpException ? exception.getResponse() : 'Internal server error',
    });
  }
}
```

Register it once, application-wide, alongside the global `ValidationPipe` — a
filter registered per-controller is easy to forget on a new one, and then that
route's errors silently fall back to Nest's default shape while every other
route matches the documented contract. A matching success-side interceptor
(wrapping `{ data, status_code, timestamp, path }` around every 2xx response)
keeps both sides of the contract symmetric, which is what a frontend or another
service actually integrates against.
