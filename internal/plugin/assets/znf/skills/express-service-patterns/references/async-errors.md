# Async error handling (Express)

<!-- Moved out of SKILL.md on 2026-09-20 to keep the skill under the 200-line budget. -->

An Express route handler that throws inside an `async` function does not
automatically reach the error-handling middleware the way a synchronous
throw does — the promise rejects and, without a catch, the request hangs.
The fix used throughout this codebase's controllers is not a wrapper
utility; it is a disciplined try/catch in every handler, ending in
`next(err)`:

```js
someAction = async (req, res, next) => {
  try {
    const result = await this.someService.doWork(req.body, req.user);
    return respond.ok(res, req, result);
  } catch (err) {
    return next(err);
  }
};
```

A custom application error carries its own status code and a stable,
machine-checkable code rather than only a human message, so the final error
middleware does not have to guess a status from a string:

```js
class AppError extends Error {
  constructor(errorCode, params = {}, statusCode = 400) {
    super(renderMessage(errorCode, params));
    this.name = 'AppError';
    this.errorCode = errorCode;
    this.statusCode = statusCode;
  }
}
```

The single error-handling middleware registered last in Middleware order
above is the only place that inspects `error.statusCode` and builds the
final JSON error body — a controller that catches an error and re-wraps it
as `new Error(err.message)` loses that status code and the stable error
code along with it, which is why a caught error should be passed to
`next(err)` unchanged rather than re-thrown as a plain `Error`.
