---
name: nestjs-patterns
description: Use when writing or changing NestJS backend code — module/controller structure, DTO + validation pipe, guards/interceptors, @InjectModel model access, DI/service layering, and consistent error envelopes.
allowed-tools: Read Grep Glob Bash
---

# NestJS patterns

## Module / controller structure

A feature module wires three things together: the Mongoose model(s) it needs via
`MongooseModule.forFeature`, its controller, and its service — and exports the
service (not the controller) so other modules can call it without importing HTTP
concerns:

```ts
@Module({
  imports: [
    MongooseModule.forFeature([
      { name: AgentGroup.name, schema: AgentGroupSchema },
      { name: User.name, schema: UserSchema },
    ]),
  ],
  controllers: [AgentGroupController],
  providers: [AgentGroupService],
  exports: [AgentGroupService],
})
export class AgentGroupModule {}
```

The controller stays thin: it decodes the request (route params, query, body via
DTOs, the current user) and delegates to the service. Business logic — validation
beyond shape, uniqueness checks, cross-collection lookups — belongs in the
service, not the controller, so it can be reused from a worker or another service
without going through HTTP:

```ts
@Controller('agent-groups')
@UseGuards(JwtAuthGuard, RolesGuard)
export class AgentGroupController {
  constructor(public service: AgentGroupService) {}

  @Post()
  create(@Body() body: CreateAgentGroupDto, @CurrentUser() user: RequestUser) {
    return this.service.create(body, user.user_id, user.tenant_id);
  }

  @Get('filter')
  filter(@Query() filter: AgentGroupFilterDto, @CurrentUser() user: RequestUser) {
    return this.service.filter(filter, user.tenant_id);
  }
}
```

## DTO + ValidationPipe

Every request body/query goes through a DTO class decorated with `class-validator`
decorators, never a bare `any` or an inline type. A global `ValidationPipe` (set up
once at bootstrap) then rejects malformed input before it reaches the controller:

```ts
app.useGlobalPipes(
  new ValidationPipe({
    whitelist: true,      // strip properties not declared on the DTO
    transform: true,      // turn query-string values into the DTO's real types
  }),
);
```

```ts
export class CreateAgentGroupDto {
  @IsString()
  @IsNotEmpty()
  name: string;

  @IsOptional()
  @IsArray()
  @IsString({ each: true })
  @IsMongoId({ each: true })
  skill_ids?: string[];

  @IsOptional()
  @IsBoolean()
  is_default?: boolean;
}
```

`whitelist: true` is what makes an unexpected field silently disappear instead of
crashing validation — pair it with `@IsOptional()` on every field that is not
required, or a legitimate partial update gets rejected. A filter/query DTO follows
the same shape as a body DTO; there is no separate mechanism for query parameters
once `transform: true` is set.

## Guard / interceptor / pipe

A guard answers yes/no before the handler runs (`canActivate`) — authentication,
role checks, feature flags. Order matters when several guards are stacked: an
authentication guard has to run before a role guard, or there is no `user` on the
request yet for the role check to read:

```ts
@UseGuards(JwtAuthGuard, RolesGuard)
```

A guard that finds nothing to check (no `@Roles()` metadata on the route) should
default to allowing the request through, not denying it — a route with no
declared requirement is not the same as a route that requires "no role at all":

```ts
@Injectable()
export class RolesGuard implements CanActivate {
  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const requiredRoles = this.reflector.getAllAndOverride<string[]>(ROLES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    if (!requiredRoles) return true; // no @Roles() declared → auth alone is enough
    const { user } = context.switchToHttp().getRequest();
    if (requiredRoles.some((role) => user?.role?.includes(role))) return true;
    throw new ForbiddenException();
  }
}
```

An interceptor wraps the handler on both sides (`intercept` around
`next.handle()`) — request-scoped logging, response shaping, caching. Keep the
transform on the success path defensive: it runs on every response, so a thrown
error inside the interceptor turns a working 200 into a 500 the caller never
asked for. A pipe transforms/validates a single argument (`ValidationPipe`,
`ParseIntPipe`) before it reaches the handler; reach for a pipe when the
concern is "is this one argument well-formed", a guard when the concern is
"should this request be allowed to happen at all".

## `@InjectModel` model access

Inject the Mongoose model by the schema class's name, not a string literal —
the string form still compiles but breaks silently if the class is renamed:

```ts
@Injectable()
export class AgentGroupService {
  constructor(
    @InjectModel(AgentGroup.name) private model: Model<AgentGroupDocument>,
    @InjectModel(User.name) private userModel: Model<UserDocument>,
  ) {}
}
```

A service that only reads a related collection (to enrich a response, to
validate a foreign id) injects that collection's model directly rather than
calling out to the other module's HTTP layer — this is the whole point of
`exports: [XService]` from the module above: internal collaborators use the DI
graph, not `fetch`. Prefer `.lean()` on read paths that only serialize the
result, and always scope a query by tenant before returning documents that
belong to more than one tenant.

## Service layering / DI

A base service class (a generic CRUD/pagination base) belongs behind
`extends`, not copy-pasted per module — a feature service adds only what is
specific to that collection on top of it:

```ts
@Injectable()
export class AgentGroupService extends BaseCrudService<AgentGroupDocument> {
  constructor(@InjectModel(AgentGroup.name) model: Model<AgentGroupDocument>) {
    super(model);
  }

  async create(data: CreateAgentGroupInput, createdBy: string, tenantId: string) {
    const existing = await this.model.findOne({ name: data.name, tenant_id: tenantId });
    if (existing) throw new ConflictException('agent-group already exists');
    return new this.model({ ...data, created_by: createdBy, tenant_id: tenantId }).save();
  }
}
```

Everything a service needs — models, other services, config — arrives through
constructor injection. A service should never reach for a global singleton or
`require()` another module's internals directly; that is what makes it
testable by swapping the injected dependency for a mock, and what keeps the
module graph (who depends on whom) visible from the `@Module()` declarations
alone.

## Error envelope

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

## Cross-refs

- An HTTP response shape consumed by another service or the frontend →
  `znf:ship`'s verification step and, for a shared contract, `znf:gate`.
- Mongoose schema/index design and safe field rollout on the model the
  `@InjectModel` above wires in → the `mongoose-modeling` skill.
- Tenant-scoping and raw-driver traps on the same collection → the
  `mongo-data-safety` skill.
