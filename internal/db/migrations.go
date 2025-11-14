package db

import (
	"fmt"

	"gorm.io/gorm"
)

var migrationStatements = []string{
	`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,
	`CREATE EXTENSION IF NOT EXISTS "pgcrypto";`,
	`CREATE EXTENSION IF NOT EXISTS "postgis";`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'cleaning_area_status') THEN
			CREATE TYPE cleaning_area_status AS ENUM ('ACTIVE', 'INACTIVE');
		END IF;
	END
	$$;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'camera_type') THEN
			CREATE TYPE camera_type AS ENUM ('LPR', 'VOLUME');
		END IF;
	END
	$$;`,
	`CREATE TABLE IF NOT EXISTS cleaning_areas (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name TEXT NOT NULL,
		description TEXT,
		geometry geometry(POLYGON, 4326) NOT NULL,
		city TEXT NOT NULL DEFAULT 'Petropavlovsk',
		status cleaning_area_status NOT NULL DEFAULT 'ACTIVE',
		default_contractor_id UUID,
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`,
	`CREATE INDEX IF NOT EXISTS idx_cleaning_areas_status ON cleaning_areas (status);`,
	`CREATE INDEX IF NOT EXISTS idx_cleaning_areas_default_contractor_id ON cleaning_areas (default_contractor_id);`,
	`CREATE INDEX IF NOT EXISTS idx_cleaning_areas_geometry ON cleaning_areas USING GIST (geometry);`,
	`CREATE TABLE IF NOT EXISTS polygons (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name TEXT NOT NULL,
		address TEXT,
		geometry geometry(POLYGON, 4326) NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`,
	`CREATE INDEX IF NOT EXISTS idx_polygons_geometry ON polygons USING GIST (geometry);`,
	`CREATE TABLE IF NOT EXISTS cameras (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		polygon_id UUID NOT NULL REFERENCES polygons(id) ON DELETE CASCADE,
		type camera_type NOT NULL,
		name TEXT NOT NULL,
		location geometry(POINT, 4326),
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`,
	`CREATE INDEX IF NOT EXISTS idx_cameras_polygon_id ON cameras (polygon_id);`,
	`CREATE INDEX IF NOT EXISTS idx_cameras_type ON cameras (type);`,
	`CREATE INDEX IF NOT EXISTS idx_cameras_location ON cameras USING GIST (location);`,
	`CREATE TABLE IF NOT EXISTS cleaning_area_access (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		cleaning_area_id UUID NOT NULL REFERENCES cleaning_areas(id) ON DELETE CASCADE,
		contractor_id UUID NOT NULL,
		source TEXT NOT NULL DEFAULT 'MANUAL',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMPTZ
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_cleaning_area_access_unique
		ON cleaning_area_access (cleaning_area_id, contractor_id);`,
	`CREATE INDEX IF NOT EXISTS idx_cleaning_area_access_contractor
		ON cleaning_area_access (contractor_id)
		WHERE revoked_at IS NULL;`,
	`CREATE TABLE IF NOT EXISTS polygon_access (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		polygon_id UUID NOT NULL REFERENCES polygons(id) ON DELETE CASCADE,
		contractor_id UUID NOT NULL,
		source TEXT NOT NULL DEFAULT 'MANUAL',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMPTZ
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_polygon_access_unique
		ON polygon_access (polygon_id, contractor_id);`,
	`CREATE INDEX IF NOT EXISTS idx_polygon_access_contractor
		ON polygon_access (contractor_id)
		WHERE revoked_at IS NULL;`,
	`CREATE OR REPLACE FUNCTION set_updated_at()
	RETURNS TRIGGER AS $$
	BEGIN
		NEW.updated_at = NOW();
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_cleaning_areas_updated_at') THEN
			CREATE TRIGGER trg_cleaning_areas_updated_at
				BEFORE UPDATE ON cleaning_areas
				FOR EACH ROW
				EXECUTE PROCEDURE set_updated_at();
		END IF;
	END
	$$;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_polygons_updated_at') THEN
			CREATE TRIGGER trg_polygons_updated_at
				BEFORE UPDATE ON polygons
				FOR EACH ROW
				EXECUTE PROCEDURE set_updated_at();
		END IF;
	END
	$$;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_cameras_updated_at') THEN
			CREATE TRIGGER trg_cameras_updated_at
				BEFORE UPDATE ON cameras
				FOR EACH ROW
				EXECUTE PROCEDURE set_updated_at();
		END IF;
	END
	$$;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_cleaning_area_access_updated_at') THEN
			CREATE TRIGGER trg_cleaning_area_access_updated_at
				BEFORE UPDATE ON cleaning_area_access
				FOR EACH ROW
				EXECUTE PROCEDURE set_updated_at();
		END IF;
	END
	$$;`,
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_polygon_access_updated_at') THEN
			CREATE TRIGGER trg_polygon_access_updated_at
				BEFORE UPDATE ON polygon_access
				FOR EACH ROW
				EXECUTE PROCEDURE set_updated_at();
		END IF;
	END
	$$;`,
}

func runMigrations(db *gorm.DB) error {
	for i, stmt := range migrationStatements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
