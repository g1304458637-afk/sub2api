import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import chat from './chat'
import draw from './draw'
import tts from './tts'
import music from './music'
import research from './research'
import admin from './admin'
import misc from './misc'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...chat,
  ...draw,
  ...tts,
  ...music,
  ...research,
  admin,
  ...misc,
}
