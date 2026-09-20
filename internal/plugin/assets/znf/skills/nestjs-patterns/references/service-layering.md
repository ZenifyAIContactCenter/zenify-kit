# Service layering / DI (NestJS)

<!-- Moved out of SKILL.md on 2026-09-20 to keep the skill under the 200-line budget. -->

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
