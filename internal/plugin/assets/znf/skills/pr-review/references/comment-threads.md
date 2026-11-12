# pr-review: review threads on the comment path

Only on the comment path: after each push (step 6.4), and after declines sent with nothing pushed (step 5.5). On the tool path nothing is posted on the PR and no thread is touched.

## 1. Who opened a thread
The bot is the app that owns the declared check, read from the check run on the head it reviewed:

```bash
gh api "repos/<owner>/<repo>/commits/<sha>/check-runs" --jq '.check_runs[] | select(.name=="<check>") | .app.slug' | head -1
```

A thread is the bot's when its first comment's author has `__typename` `Bot` and `login` equal to that slug (GraphQL gives the bare slug; also accept `<slug>[bot]`). Every other thread belongs to a person.

## 2. List the open threads

```bash
gh api graphql --paginate -f o=<owner> -f r=<repo> -F n=<n> -f query='query($o:String!,$r:String!,$n:Int!,$endCursor:String){repository(owner:$o,name:$r){pullRequest(number:$n){reviewThreads(first:100,after:$endCursor){nodes{id isResolved path line comments(first:1){nodes{author{__typename login} body url}}} pageInfo{hasNextPage endCursor}}}}}'
```

Keep the nodes with `isResolved: false`.

## 3. Answer each one
- The push fixed it → reply with the commit sha and one line on what changed. A bot thread: then resolve it. A person's thread: never resolve it, that is theirs.
- Its finding was declined → reply once with the reason (the decline line itself goes where the declaration says). A bot thread: then resolve it.
- The push did not touch it → leave it as it is; no reply.

```bash
gh api graphql -f t=<thread id> -f b=<reply> -f query='mutation($t:ID!,$b:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$t,body:$b}){comment{url}}}'
gh api graphql -f t=<thread id> -f query='mutation($t:ID!){resolveReviewThread(input:{threadId:$t}){thread{isResolved}}}'
```

A reply or resolve that fails → report the thread URL and go on; it never blocks the loop or costs a round.
