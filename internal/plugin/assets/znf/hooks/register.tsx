import type { Register } from 'claude-code'

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'nonstop-pane',
      description: 'Show the znf:nonstop board in a pane',
    })

    return next(e)
  })

  on('command.run', { command: 'nonstop-pane' }, async () => {
    return { text: 'nonstop pane: not implemented' }
  })
}
