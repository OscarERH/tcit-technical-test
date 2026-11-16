// Package store implements the posts repository on Cloud Spanner.
package store

import (
	"context"
	"fmt"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// schema uses snake_case identifiers. Primary keys are UUIDs (monotonic keys
// create hotspots in Spanner) and attachments are interleaved in their post so
// they are stored together and removed with it.
var schema = []string{
	`CREATE TABLE posts (
		post_id     STRING(36)  NOT NULL,
		name        STRING(100) NOT NULL,
		description STRING(500) NOT NULL,
		status      STRING(32)  NOT NULL,
		created_at  TIMESTAMP   NOT NULL OPTIONS (allow_commit_timestamp = true)
	) PRIMARY KEY (post_id)`,
	`CREATE TABLE post_attachments (
		post_id       STRING(36)   NOT NULL,
		attachment_id STRING(36)   NOT NULL,
		file_name     STRING(255)  NOT NULL,
		mime_type     STRING(255)  NOT NULL,
		drive_file_id STRING(255)  NOT NULL,
		web_view_link STRING(2048) NOT NULL,
		created_at    TIMESTAMP    NOT NULL OPTIONS (allow_commit_timestamp = true)
	) PRIMARY KEY (post_id, attachment_id),
	  INTERLEAVE IN PARENT posts ON DELETE CASCADE`,
}

// Setup creates the instance, the database and the schema when they do not
// exist yet. The emulator starts empty on every boot, so the API runs this on
// startup; against real Spanner the infrastructure is expected to be provisioned.
func Setup(ctx context.Context, project, instanceID, databaseID string) error {
	projectPath := "projects/" + project
	instancePath := fmt.Sprintf("%s/instances/%s", projectPath, instanceID)

	instances, err := instance.NewInstanceAdminClient(ctx)
	if err != nil {
		return fmt.Errorf("creating instance admin client: %w", err)
	}
	defer instances.Close()

	instanceOp, err := instances.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     projectPath,
		InstanceId: instanceID,
		Instance: &instancepb.Instance{
			Config:      projectPath + "/instanceConfigs/emulator-config",
			DisplayName: instanceID,
			NodeCount:   1,
		},
	})
	if err == nil {
		_, err = instanceOp.Wait(ctx)
	}
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("creating instance: %w", err)
	}

	databases, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		return fmt.Errorf("creating database admin client: %w", err)
	}
	defer databases.Close()

	databaseOp, err := databases.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          instancePath,
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%s`", databaseID),
		ExtraStatements: schema,
	})
	if err == nil {
		_, err = databaseOp.Wait(ctx)
	}
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("creating database: %w", err)
	}
	return nil
}
