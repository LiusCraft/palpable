/* 由 make gen 生成，来源 protocol/palpable.schema.json，请勿手改 */

/**
 * 可感设备协议（文本帧）。音频为独立的二进制 Opus 帧，不在此 schema 内。设备↔服务端所有 JSON 消息都必须匹配 oneOf 中的一项。
 */
export type PalpableMessage =
  | Hello
  | Wake
  | MicState
  | Proximity
  | Location
  | Mcp
  | HelloAck
  | AudioDownlink
  | WakeConfig
  | Light
  | Haptic
  | Sound
  | SpeakerState
  | Presence
  | ChannelState
  | FriendState
  | Error;
/**
 * 设备 MAC，小写、冒号分隔；也是设备在灵矽侧的 Device-Id
 */
export type MAC = string;
export type Capability = "mic" | "speaker" | "light" | "haptic" | "proximity" | "location" | "wake" | "mcp";
export type LightState = "idle" | "listening" | "thinking" | "speaking" | "peer_talking" | "muted";

export interface Hello {
  type: "hello";
  device_id: MAC;
  /**
   * 皮套款式，服务端据此选风格智能体；缺省或未知时用默认风格
   */
  skin?: string;
  firmware?: string;
  audio_params: AudioParams;
  capabilities: Capability[];
}
export interface AudioParams {
  format: "opus";
  sample_rate: 16000;
  channels: 1;
  /**
   * 单帧毫秒数
   */
  frame_duration: 60;
}
export interface Wake {
  type: "wake";
  source: "voice" | "touch";
}
export interface MicState {
  type: "mic.state";
  state: "start" | "stop";
}
export interface Proximity {
  type: "proximity";
  /**
   * @maxItems 16
   */
  peers:
    | []
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ]
    | [
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        },
        {
          device_id: string;
          /**
           * 归一化信号强度，越大越近
           */
          strength: number;
        }
      ];
}
export interface Location {
  type: "location";
  lat: number;
  lng: number;
}
/**
 * 设备端 MCP，双向。payload 为 JSON-RPC 2.0 消息，由服务端代理与灵矽之间转发
 */
export interface Mcp {
  type: "mcp";
  payload: {};
}
export interface HelloAck {
  type: "hello.ack";
  session_id: string;
  audio_params: AudioParams;
}
/**
 * 下行音频参数。AI 通道就绪后发送，早于任何下行音频帧；设备按它配置 Opus 解码器。
 */
export interface AudioDownlink {
  type: "audio.downlink";
  sample_rate: 16000 | 24000;
  /**
   * 单帧毫秒数
   */
  frame_duration: 20 | 40 | 60;
}
/**
 * 设备本地唤醒词引擎要识别的词（孩子给小伙伴起的名字）。上线时和起名后下发，整体替换旧的；空数组表示暂无唤醒词。
 */
export interface WakeConfig {
  type: "wake.config";
  /**
   * @maxItems 4
   */
  words: [] | [string] | [string, string] | [string, string, string] | [string, string, string, string];
}
export interface Light {
  type: "light";
  state: LightState;
  color?: string;
  pattern?: "solid" | "breathe" | "pulse";
}
export interface Haptic {
  type: "haptic";
  pattern: "tap" | "double_tap" | "long";
}
export interface Sound {
  type: "sound";
  name: "connected" | "joined" | "left" | "switched" | "error" | "busy" | "friend";
}
export interface SpeakerState {
  type: "speaker.state";
  state: "start" | "stop";
  source: "ai" | "peer";
  /**
   * source 为 peer 时的发言设备
   */
  from?: string;
  /**
   * source 为 peer 时发言者小伙伴的名字，供设备展示
   */
  name?: string;
}
export interface Presence {
  type: "presence";
  event: "join" | "leave";
  device_id: string;
  /**
   * 玩偶的名字，供设备播报
   */
  name?: string;
  channel: string;
}
/**
 * 前台频道变化；设备据此切换灯色并提示
 */
export interface ChannelState {
  type: "channel.state";
  event: "joined" | "switched" | "left";
  channel: string;
  name: string;
  color?: string;
}
/**
 * 好友关系变化；pending 时设备振动并提示，确认由家长端完成
 */
export interface FriendState {
  type: "friend.state";
  event: "pending" | "confirmed" | "removed";
  device_id: MAC;
  name?: string;
}
export interface Error {
  type: "error";
  code: "bad_message" | "unsupported" | "unauthorized" | "internal";
  message: string;
}
