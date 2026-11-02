-- 设备封锁：厂家后台挂失（locked）或吊销（revoked）后，设备不能再连接。
-- 与 device_key、device 分开：封锁不随解绑或擦除而消失；吊销的设备连 open 模式的首次登记也不能再用。
CREATE TABLE device_block (
    device_id  TEXT PRIMARY KEY,
    reason     TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);
