import { useState } from 'react';

import {
  type LogicalBackupConfig,
  logicalBackupConfigApi,
  logicalBackupsApi,
} from '../../../entity/backups/logical';
import {
  type PhysicalBackupConfig,
  physicalBackupConfigApi,
  physicalBackupsApi,
} from '../../../entity/backups/physical';
import {
  type Database,
  DatabaseType,
  type MariadbDatabase,
  type MongodbDatabase,
  type MysqlDatabase,
  Period,
  PhysicalDatabaseBackupType,
  PostgresSslMode,
  type PostgresqlLogicalDatabase,
  type PostgresqlPhysicalDatabase,
  databaseApi,
} from '../../../entity/databases';
import type { UserProfile } from '../../../entity/users';
import { EditLogicalBackupConfigComponent } from '../../backups/logical';
import { EditPhysicalBackupConfigComponent } from '../../backups/physical';
import { CreateReadOnlyComponent } from './edit/CreateReadOnlyComponent';
import { EditDatabaseBaseInfoComponent } from './edit/EditDatabaseBaseInfoComponent';
import { EditDatabaseNotifiersComponent } from './edit/EditDatabaseNotifiersComponent';
import { EditDatabaseSpecificDataComponent } from './edit/EditDatabaseSpecificDataComponent';

interface Props {
  user: UserProfile;
  workspaceId: string;
  onCreated: (databaseId: string) => void;
  onClose: () => void;
}

const createInitialDatabase = (workspaceId: string): Database =>
  ({
    id: undefined as unknown as string,
    name: '',
    workspaceId,
    storePeriod: Period.MONTH,

    type: DatabaseType.POSTGRES_LOGICAL,

    storage: {} as unknown as Storage,

    notifiers: [],
    sendNotificationsOn: [],
  }) as Database;

const initializeDatabaseTypeData = (db: Database): Database => {
  const base = {
    ...db,
    postgresql: undefined,
    postgresqlPhysical: undefined,
    mysql: undefined,
    mariadb: undefined,
    mongodb: undefined,
  };

  switch (db.type) {
    case DatabaseType.POSTGRES_LOGICAL:
      return {
        ...base,
        postgresqlLogical:
          db.postgresqlLogical ??
          ({
            cpuCount: 1,
          } as PostgresqlLogicalDatabase),
      };
    case DatabaseType.POSTGRES_PHYSICAL:
      return {
        ...base,
        postgresqlPhysical:
          db.postgresqlPhysical ??
          ({
            backupType: PhysicalDatabaseBackupType.FULL,
            sslMode: PostgresSslMode.Disable,
          } as PostgresqlPhysicalDatabase),
      };
    case DatabaseType.MYSQL:
      return { ...base, mysql: db.mysql ?? ({} as MysqlDatabase) };
    case DatabaseType.MARIADB:
      return { ...base, mariadb: db.mariadb ?? ({} as MariadbDatabase) };
    case DatabaseType.MONGODB:
      return { ...base, mongodb: db.mongodb ?? ({ cpuCount: 1 } as MongodbDatabase) };
    default:
      return db;
  }
};

export const CreateDatabaseComponent = ({ user, workspaceId, onCreated, onClose }: Props) => {
  const [isCreating, setIsCreating] = useState(false);
  const [backupConfig, setBackupConfig] = useState<LogicalBackupConfig | undefined>();
  const [physicalBackupConfig, setPhysicalBackupConfig] = useState<
    PhysicalBackupConfig | undefined
  >();
  const [database, setDatabase] = useState<Database>(createInitialDatabase(workspaceId));

  const [step, setStep] = useState<
    'base-info' | 'db-settings' | 'create-readonly-user' | 'backup-config' | 'notifiers'
  >('base-info');

  const isPhysical = database.type === DatabaseType.POSTGRES_PHYSICAL;

  const createDatabase = async (database: Database) => {
    if (isPhysical ? !physicalBackupConfig : !backupConfig) return;

    setIsCreating(true);

    try {
      const createdDatabase = await databaseApi.createDatabase(database);
      setDatabase({ ...createdDatabase });

      if (isPhysical && physicalBackupConfig) {
        physicalBackupConfig.databaseId = createdDatabase.id;
        await physicalBackupConfigApi.savePhysicalBackupConfig(physicalBackupConfig);

        if (physicalBackupConfig.isBackupsEnabled) {
          await physicalBackupsApi.triggerPhysicalBackup(createdDatabase.id, 'auto');
        }
      } else if (backupConfig) {
        backupConfig.databaseId = createdDatabase.id;
        await logicalBackupConfigApi.saveBackupConfig(backupConfig);

        if (backupConfig.isBackupsEnabled) {
          await logicalBackupsApi.makeBackup(createdDatabase.id);
        }
      }

      onCreated(createdDatabase.id);
      onClose();
    } catch (error) {
      alert(error);
    }

    setIsCreating(false);
  };

  if (step === 'base-info') {
    return (
      <div>
        <EditDatabaseBaseInfoComponent
          database={database}
          isShowName
          isShowType
          isSaveToApi={false}
          saveButtonText="Continue"
          onCancel={() => onClose()}
          onSaved={(db) => {
            const initializedDb = initializeDatabaseTypeData(db);
            setDatabase({ ...initializedDb });
            setStep('db-settings');
          }}
        />
      </div>
    );
  }

  if (step === 'db-settings') {
    return (
      <EditDatabaseSpecificDataComponent
        database={database}
        isShowCancelButton={false}
        onCancel={() => onClose()}
        isShowBackButton
        onBack={() => setStep('base-info')}
        saveButtonText="Continue"
        isSaveToApi={false}
        onSaved={(database) => {
          setDatabase({ ...database });
          setStep('create-readonly-user');
        }}
      />
    );
  }

  if (step === 'create-readonly-user') {
    return (
      <CreateReadOnlyComponent
        database={database}
        onReadOnlyUserUpdated={(database) => {
          setDatabase({ ...database });
          setStep('backup-config');
        }}
        onGoBack={() => setStep('db-settings')}
        onSkipped={() => setStep('backup-config')}
        onAlreadyExists={() => setStep('backup-config')}
      />
    );
  }

  if (step === 'backup-config') {
    if (isPhysical) {
      return (
        <EditPhysicalBackupConfigComponent
          user={user}
          database={database}
          isShowCancelButton={false}
          onCancel={() => onClose()}
          isShowBackButton
          onBack={() => setStep('db-settings')}
          saveButtonText="Continue"
          isSaveToApi={false}
          onSaved={(physicalBackupConfig) => {
            setPhysicalBackupConfig(physicalBackupConfig);
            setStep('notifiers');
          }}
        />
      );
    }

    return (
      <EditLogicalBackupConfigComponent
        user={user}
        database={database}
        isShowCancelButton={false}
        onCancel={() => onClose()}
        isShowBackButton
        onBack={() => setStep('db-settings')}
        saveButtonText="Continue"
        isSaveToApi={false}
        onSaved={(backupConfig) => {
          setBackupConfig(backupConfig);
          setStep('notifiers');
        }}
      />
    );
  }

  if (step === 'notifiers') {
    if (isCreating) {
      return <div>Creating database...</div>;
    }

    return (
      <EditDatabaseNotifiersComponent
        database={database}
        isShowCancelButton={false}
        workspaceId={workspaceId}
        onCancel={() => onClose()}
        isShowBackButton
        onBack={() => setStep('backup-config')}
        isShowSaveOnlyForUnsaved={false}
        saveButtonText="Complete"
        isSaveToApi={false}
        onSaved={(database) => {
          if (isCreating) return;

          setDatabase({ ...database });
          createDatabase(database);
        }}
      />
    );
  }
};
