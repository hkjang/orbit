-- Anchored Star: 가족·멘토처럼 교류 빈도와 무관하게 곁에 두어야 하는 관계는
-- 활동이 줄어도 Outer Orbit이나 Dark Orbit으로 밀려나지 않습니다.
ALTER TABLE relationships ADD COLUMN IF NOT EXISTS anchored boolean NOT NULL DEFAULT false;
