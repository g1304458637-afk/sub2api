-- 音乐生成显式定价：按首计费。
-- NULL = 使用代码默认单价（defaultMusicPricePerTrack = 0.5）；显式 0 = 免费；>0 = 分组覆盖价。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS music_price_per_track DECIMAL(20,8);
