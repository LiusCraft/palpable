-- 小伙伴进频道的家长控制，见 docs/trust.md §5。
-- agent_in_channel：自家家长是否允许自家小伙伴在频道里发言，默认允许（只在主人点名时开口）。
-- blocked_agents：自家家长屏蔽的别家小伙伴（主人设备 MAC，逗号分隔）；屏蔽后自家孩子听不到它。
ALTER TABLE device ADD COLUMN agent_in_channel INTEGER NOT NULL DEFAULT 1;
ALTER TABLE device ADD COLUMN blocked_agents   TEXT    NOT NULL DEFAULT '';
