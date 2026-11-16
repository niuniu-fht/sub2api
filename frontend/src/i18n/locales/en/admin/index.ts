import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'
import imageBilling from './imageBilling'
import plugins from './plugins'
import profit from './profit'

export default {
  ...overview,
  ...profit,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  ...imageBilling,
  ...plugins,
}
