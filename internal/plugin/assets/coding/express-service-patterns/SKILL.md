---
name: express-service-patterns
description: Use when writing or changing Express (legacy) backend code — controller/service layering, a central model-registry access pattern, a unified response-envelope helper, middleware, and async error handling.
allowed-tools: Read Grep Glob Bash
---

# Express service patterns

## Controller / service layering

A controller method decodes the request (params, query, body, the
authenticated user) and delegates to a service; it never runs a query or a
business rule itself. Every handler wraps its body in try/catch and forwards
a caught error to `next(err)` rather than responding directly — that keeps
error formatting in exactly one place (see Async error handling below):

```js
class OrganizationController {
  constructor() {
    this.organizationService = new OrganizationService();
  }

  importWithFile = async (req, res, next) => {
    try {
      const result = await this.organizationService.importWithFile(
        req.file,
        req.user,
        req.body?.map_header,
      );
      return respond.ok(res, req, result);
    } catch (err) {
      logger.error('OrganizationController#importWithFile failed', {
        message: err?.message,
      });
      return next(err);
    }
  };
}
```

Business logic — validation beyond shape, uniqueness checks, cross-collection
lookups, anything that needs to be reusable from a queue worker rather than
only from HTTP — belongs in the service. The service takes plain arguments
(the file, the current user, an already-validated body) rather than the raw
`req`/`res` pair, so it can be called from anywhere, not just from a route.

## Model-registry access (agnostic)

Data-layer clients (Mongo models, SQL connections, a search client) are not
`require`d individually at each call site. They are assembled once into a
single registry object and every service reaches through that one entry
point:

```js
// db/index.js — assembled once
module.exports = {
  mongo: require('./mongo'),   // e.g. db.mongo.organization, db.mongo.ticket
  sql: require('./sql'),
  search: require('./search'),
};
```

```js
// a service — never requires ./mongo directly
const db = require('../../db');

class OrganizationService {
  async findByPhone(phone, tenantId) {
    return db.mongo.organization.findOne({phone, tenant_id: tenantId});
  }
}
```

The payoff is that swapping or mocking the underlying client happens in one
file, and a service's dependencies are visible from its `require('../../db')`
line rather than scattered across a dozen individual model imports. When
adding a new collection/table client, register it in the same assembly file
next to the others rather than reaching for a fresh ad-hoc `require` in the
service that first needs it — a second service that needs the same client
later will otherwise re-`require` the raw module and silently bypass the
registry.

## Response-envelope helper

Every successful response goes through one small helper rather than each
controller calling `res.json(...)` with its own shape. That helper is the
only place that knows the success envelope, and the only place a tracking
side-effect (metrics, audit) needs to be wired in:

```js
// respond/ok.js
module.exports = function (res, req, data, meta, message) {
  requestTracker.success(req);
  res.status(200).json({
    error: false,
    status: 200,
    data: data ?? null,
    meta,
    message,
  });
};
```

```js
// respond/index.js
module.exports = {
  ok: require('./ok'),
  badRequest: require('./badRequest'),
  notFound: require('./notFound'),
  forbidden: require('./forbidden'),
};
```

A controller calls `respond.ok(res, req, data)`, never `res.json(...)`
directly on a success path — a handler that reaches for a bare `res.json`
produces a response shape the frontend's shared response interceptor does
not recognize, which is a much harder bug to spot than a missing field. Keep
error responses on the same discipline: route every thrown/caught error
through `next(err)` and let one error-formatting middleware (below) build
the error envelope, rather than letting individual handlers each invent
their own error JSON shape.

## Middleware order

Route registration order is the actual authorization boundary in an app
without a decorator-based guard system — a route registered before the auth
middleware runs with no `req.user` at all:

```js
app.use(requestTrackerMiddleware);
app.use(internalRoutes);     // trusted network only, no end-user auth needed
app.use(publicRoutes);       // no auth required by design
app.use(webhookRoutes);
app.use(apiRoutes);
app.use(authMiddleware);     // attaches req.user; everything after this line is protected
app.use(scopeMiddleware);    // needs req.user — must come after auth
app.use(privateRoutes);      // authenticated app routes
app.use(errorHandlerMiddleware); // always last
```

Two things to check whenever a new middleware is inserted: nothing after
`authMiddleware` may run before it (an out-of-order insert silently makes a
route open), and any middleware reading a value another middleware attaches
to `req` (`req.user`, `req.scope`) must be registered after the one that
sets it, not just imported after it — import order and registration order
are independent in this style of app and only registration order matters at
request time.

## Async error handling

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

## Cross-refs

- The Mongo/Mongoose model behind `db.mongo.*` above — schema and index
  design, safe field rollout — → the `mongoose-modeling` skill.
- Tenant-scoping and raw-driver traps on the same collection → the
  `mongo-data-safety` skill.
- The NestJS-side equivalent of controller/service layering and its error
  envelope, for the newer half of a system that runs both stacks side by
  side → the `nestjs-patterns` skill.
- A response shape consumed by a frontend or another service →
  `znf:ship`'s verification step and, for a shared contract, `znf:gate`.
