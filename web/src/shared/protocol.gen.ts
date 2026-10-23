/* 由 make gen 生成，来源 protocol/palpable.schema.json，请勿手改 */

/**
 * 可感设备协议（文本帧）。音频为独立的二进制 Opus 帧，不在此 schema 内。设备↔服务端所有 JSON 消息都必须匹配 oneOf 中的一项。
 */
export type PalpableMessage =
  Hello | Wake | MicState | Proximity | Location | HelloAck | Light | Haptic | Sound | SpeakerState | Presence | Error;
export type Capability = "mic" | "speaker" | "light" | "haptic" | "proximity" | "location" | "wake";
export type LightState = "idle" | "listening" | "thinking" | "speaking" | "peer_talking" | "muted";

export interface Hello {
  type: "hello";
  device_id: string;
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
export interface HelloAck {
  type: "hello.ack";
  session_id: string;
  audio_params: AudioParams;
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
  name: "connected" | "joined" | "left" | "switched" | "error";
}
export interface SpeakerState {
  type: "speaker.state";
  state: "start" | "stop";
  source: "ai" | "peer";
  /**
   * source 为 peer 时的发言设备
   */
  from?: string;
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
export interface Error {
  type: "error";
  code: "bad_message" | "unsupported" | "unauthorized" | "internal";
  message: string;
}
