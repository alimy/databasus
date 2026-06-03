import { CopyOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { App, Button, Input, InputNumber, Select, Tooltip } from 'antd';
import { useEffect, useState } from 'react';

import { IS_CLOUD } from '../../../../constants';
import {
  type Database,
  PhysicalDatabaseBackupType,
  PostgresSslMode,
  type PostgresqlPhysicalDatabase,
  PostgresqlVersion,
  databaseApi,
} from '../../../../entity/databases';
import { ConnectionStringParser } from '../../../../entity/databases/model/postgresql/ConnectionStringParser';
import { ClipboardHelper } from '../../../../shared/lib/ClipboardHelper';
import { ToastHelper } from '../../../../shared/toast';
import { ClipboardPasteModalComponent } from '../../../../shared/ui';

interface Props {
  database: Database;

  isShowCancelButton?: boolean;
  onCancel: () => void;

  isShowBackButton: boolean;
  onBack: () => void;

  saveButtonText?: string;
  isSaveToApi: boolean;
  onSaved: (database: Database) => void;

  isShowDbName?: boolean;
  isRestoreMode?: boolean;
}

const IPV4_PATTERN = /^\d{1,3}(\.\d{1,3}){3}$/;

const postgresqlVersionLabels: Record<string, string> = {
  [PostgresqlVersion.PostgresqlVersion12]: '12',
  [PostgresqlVersion.PostgresqlVersion13]: '13',
  [PostgresqlVersion.PostgresqlVersion14]: '14',
  [PostgresqlVersion.PostgresqlVersion15]: '15',
  [PostgresqlVersion.PostgresqlVersion16]: '16',
  [PostgresqlVersion.PostgresqlVersion17]: '17',
  [PostgresqlVersion.PostgresqlVersion18]: '18',
};

const deriveSslModeFromHost = (rawHost: string): PostgresSslMode | null => {
  const trimmed = rawHost.trim().toLowerCase();

  if (trimmed.startsWith('https://')) return PostgresSslMode.Require;
  if (trimmed.startsWith('http://')) return PostgresSslMode.Disable;

  const bareHost = trimmed.split(':')[0];
  if (bareHost === 'localhost' || IPV4_PATTERN.test(bareHost)) {
    return PostgresSslMode.Disable;
  }

  return null;
};

const applySslMode = (
  postgresqlPhysical: PostgresqlPhysicalDatabase,
  sslMode: PostgresSslMode,
): PostgresqlPhysicalDatabase => {
  if (sslMode === PostgresSslMode.Disable) {
    return {
      ...postgresqlPhysical,
      sslMode,
      sslClientCert: '',
      sslClientKey: '',
      sslRootCert: '',
    };
  }

  return { ...postgresqlPhysical, sslMode };
};

export const EditPostgreSqlPhysicalSpecificDataComponent = ({
  database,

  isShowCancelButton,
  onCancel,

  isShowBackButton,
  onBack,

  saveButtonText,
  isSaveToApi,
  onSaved,
}: Props) => {
  const { message } = App.useApp();

  const [editingDatabase, setEditingDatabase] = useState<Database>();
  const [isSaving, setIsSaving] = useState(false);

  const [isConnectionTested, setIsConnectionTested] = useState(false);
  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [isConnectionFailed, setIsConnectionFailed] = useState(false);

  const [hasUserChosenSslMode, setHasUserChosenSslMode] = useState(!!database.id);
  const [isReplacingCerts, setIsReplacingCerts] = useState(false);

  const [isShowPasteModal, setIsShowPasteModal] = useState(false);

  const applyConnectionString = (text: string) => {
    const trimmedText = text.trim();

    if (!trimmedText) {
      message.error('Clipboard is empty');
      return;
    }

    const result = ConnectionStringParser.parse(trimmedText);

    if ('error' in result) {
      message.error(result.error);
      return;
    }

    if (!editingDatabase?.postgresqlPhysical) return;

    const updatedDatabase: Database = {
      ...editingDatabase,
      postgresqlPhysical: {
        ...editingDatabase.postgresqlPhysical,
        host: result.host,
        port: result.port,
        username: result.username,
        password: result.password,
        sslMode: result.sslMode,
      },
    };

    setHasUserChosenSslMode(true);
    setEditingDatabase(updatedDatabase);
    setIsConnectionTested(false);
    message.success('Connection string parsed successfully');
  };

  const parseFromClipboard = async () => {
    if (!ClipboardHelper.isClipboardApiAvailable()) {
      setIsShowPasteModal(true);
      return;
    }

    try {
      const text = await ClipboardHelper.readFromClipboard();
      applyConnectionString(text);
    } catch {
      message.error('Failed to read clipboard. Please check browser permissions.');
    }
  };

  const updateBackupType = (backupType: PhysicalDatabaseBackupType) => {
    if (!editingDatabase?.postgresqlPhysical) return;

    setEditingDatabase({
      ...editingDatabase,
      postgresqlPhysical: { ...editingDatabase.postgresqlPhysical, backupType },
    });
    setIsConnectionTested(false);
  };

  const testConnection = async () => {
    if (!editingDatabase?.postgresqlPhysical) return;
    setIsTestingConnection(true);
    setIsConnectionFailed(false);

    const trimmedDatabase = {
      ...editingDatabase,
      postgresqlPhysical: {
        ...editingDatabase.postgresqlPhysical,
        password: editingDatabase.postgresqlPhysical.password?.trim(),
      },
    };

    try {
      await databaseApi.testDatabaseConnectionDirect(trimmedDatabase);
      setIsConnectionTested(true);
      ToastHelper.showToast({
        title: 'Connection test passed',
        description: 'You can continue with the next step',
      });
    } catch (e) {
      setIsConnectionFailed(true);
      alert((e as Error).message);
    }

    setIsTestingConnection(false);
  };

  const saveDatabase = async () => {
    if (!editingDatabase?.postgresqlPhysical) return;

    const trimmedDatabase = {
      ...editingDatabase,
      postgresqlPhysical: {
        ...editingDatabase.postgresqlPhysical,
        password: editingDatabase.postgresqlPhysical.password?.trim(),
      },
    };

    if (isSaveToApi) {
      setIsSaving(true);

      try {
        await databaseApi.updateDatabase(trimmedDatabase);
      } catch (e) {
        alert((e as Error).message);
      }

      setIsSaving(false);
    }

    onSaved(trimmedDatabase);
  };

  const updatePostgresqlCert = (
    field: 'sslClientCert' | 'sslClientKey' | 'sslRootCert',
    value: string,
  ) => {
    if (!editingDatabase?.postgresqlPhysical) return;

    setEditingDatabase({
      ...editingDatabase,
      postgresqlPhysical: { ...editingDatabase.postgresqlPhysical, [field]: value },
    });
    setIsConnectionTested(false);
  };

  const startReplacingCerts = () => {
    if (!editingDatabase?.postgresqlPhysical) return;

    setIsReplacingCerts(true);
    setEditingDatabase({
      ...editingDatabase,
      postgresqlPhysical: {
        ...editingDatabase.postgresqlPhysical,
        sslClientCert: '',
        sslClientKey: '',
        sslRootCert: '',
      },
    });
    setIsConnectionTested(false);
  };

  useEffect(() => {
    setIsSaving(false);
    setIsConnectionTested(false);
    setIsTestingConnection(false);
    setIsConnectionFailed(false);
    setIsReplacingCerts(false);
    setHasUserChosenSslMode(!!database.id);

    setEditingDatabase({ ...database });
  }, [database]);

  if (!editingDatabase) return null;

  const backupTypeOptions = [
    { label: 'Full backups only', value: PhysicalDatabaseBackupType.FULL },
    { label: 'Full + incremental', value: PhysicalDatabaseBackupType.FULL_INCREMENTAL },
    ...(IS_CLOUD
      ? []
      : [
          {
            label: 'Full + incremental + WAL streaming',
            value: PhysicalDatabaseBackupType.FULL_INCREMENTAL_WAL_STREAM,
          },
        ]),
  ];

  const renderFooter = (footerContent?: React.ReactNode) => (
    <div className="mt-5 flex">
      {isShowCancelButton && (
        <Button className="mr-1" danger ghost onClick={() => onCancel()}>
          Cancel
        </Button>
      )}

      {isShowBackButton && (
        <Button className="mr-auto" type="primary" ghost onClick={() => onBack()}>
          Back
        </Button>
      )}

      {footerContent}
    </div>
  );

  const renderSslCertSection = () => {
    const sslMode = editingDatabase.postgresqlPhysical?.sslMode ?? PostgresSslMode.Disable;
    if (sslMode === PostgresSslMode.Disable) return null;

    const hadSslCert = !!database.postgresqlPhysical?.sslClientCert;
    if (hadSslCert && !isReplacingCerts) {
      return (
        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">Client certificate</div>
          <div className="flex items-center">
            <span className="mr-3">*************</span>
            <Button size="small" onClick={startReplacingCerts}>
              Replace
            </Button>
          </div>
        </div>
      );
    }

    return (
      <>
        <div className="mb-1 flex w-full items-start">
          <div className="min-w-[150px]">Client certificate</div>
          <Input.TextArea
            value={editingDatabase.postgresqlPhysical?.sslClientCert || ''}
            onChange={(e) => updatePostgresqlCert('sslClientCert', e.target.value)}
            size="small"
            className="max-w-[300px] grow"
            placeholder="-----BEGIN CERTIFICATE-----"
            autoSize={{ minRows: 2, maxRows: 5 }}
          />
        </div>

        <div className="mb-1 flex w-full items-start">
          <div className="min-w-[150px]">Client key</div>
          <Input.TextArea
            value={editingDatabase.postgresqlPhysical?.sslClientKey || ''}
            onChange={(e) => updatePostgresqlCert('sslClientKey', e.target.value)}
            size="small"
            className="max-w-[300px] grow"
            placeholder="-----BEGIN PRIVATE KEY-----"
            autoSize={{ minRows: 2, maxRows: 5 }}
          />
        </div>

        <div className="mb-1 flex w-full items-start">
          <div className="flex min-w-[150px] items-center">
            <span>Server CA certificate</span>
            <Tooltip
              className="cursor-pointer"
              title="Optional. When provided, the server certificate is verified against this CA (verify-ca / verify-full)."
            >
              <InfoCircleOutlined className="ml-2" style={{ color: 'gray' }} />
            </Tooltip>
          </div>
          <Input.TextArea
            value={editingDatabase.postgresqlPhysical?.sslRootCert || ''}
            onChange={(e) => updatePostgresqlCert('sslRootCert', e.target.value)}
            size="small"
            className="max-w-[300px] grow"
            placeholder="-----BEGIN CERTIFICATE-----"
            autoSize={{ minRows: 2, maxRows: 5 }}
          />
        </div>
      </>
    );
  };

  const renderForm = () => {
    let isAllFieldsFilled = true;
    if (!editingDatabase.postgresqlPhysical?.host) isAllFieldsFilled = false;
    if (!editingDatabase.postgresqlPhysical?.port) isAllFieldsFilled = false;
    if (!editingDatabase.postgresqlPhysical?.username) isAllFieldsFilled = false;
    if (!editingDatabase.id && !editingDatabase.postgresqlPhysical?.password)
      isAllFieldsFilled = false;

    const detectedVersion = editingDatabase.postgresqlPhysical?.version;

    return (
      <>
        <div className="mb-3 flex w-full items-center">
          <div className="min-w-[150px]">Backup type</div>
          <Select
            value={
              editingDatabase.postgresqlPhysical?.backupType ?? PhysicalDatabaseBackupType.FULL
            }
            onChange={updateBackupType}
            options={backupTypeOptions}
            size="small"
            className="max-w-[250px] grow"
          />
        </div>

        <div className="mb-3 flex">
          <div className="min-w-[150px]" />
          <div
            className="cursor-pointer text-sm text-gray-600 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
            onClick={parseFromClipboard}
          >
            <CopyOutlined className="mr-1" />
            Parse from clipboard
          </div>
        </div>

        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">Host</div>
          <Input
            value={editingDatabase.postgresqlPhysical?.host}
            onChange={(e) => {
              if (!editingDatabase.postgresqlPhysical) return;

              const rawHost = e.target.value;
              const basePostgresql = {
                ...editingDatabase.postgresqlPhysical,
                host: rawHost.trim().replace('https://', '').replace('http://', ''),
              };
              const isHttpsHost = rawHost.trim().toLowerCase().startsWith('https://');
              const currentSslMode = basePostgresql.sslMode ?? PostgresSslMode.Disable;

              let derivedSslMode: PostgresSslMode | null = null;
              if (hasUserChosenSslMode) {
                if (isHttpsHost && currentSslMode === PostgresSslMode.Disable) {
                  derivedSslMode = PostgresSslMode.Require;
                }
              } else {
                derivedSslMode = deriveSslModeFromHost(rawHost);
              }

              setEditingDatabase({
                ...editingDatabase,
                postgresqlPhysical:
                  derivedSslMode !== null
                    ? applySslMode(basePostgresql, derivedSslMode)
                    : basePostgresql,
              });
              setIsConnectionTested(false);
            }}
            size="small"
            className="max-w-[200px] grow"
            placeholder="Enter PG host"
          />
        </div>

        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">Port</div>
          <InputNumber
            type="number"
            value={editingDatabase.postgresqlPhysical?.port}
            onChange={(e) => {
              if (!editingDatabase.postgresqlPhysical || e === null) return;

              setEditingDatabase({
                ...editingDatabase,
                postgresqlPhysical: { ...editingDatabase.postgresqlPhysical, port: e },
              });
              setIsConnectionTested(false);
            }}
            size="small"
            className="max-w-[200px] grow"
            placeholder="Enter PG port"
          />
        </div>

        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">Username</div>
          <Input
            value={editingDatabase.postgresqlPhysical?.username}
            onChange={(e) => {
              if (!editingDatabase.postgresqlPhysical) return;

              setEditingDatabase({
                ...editingDatabase,
                postgresqlPhysical: {
                  ...editingDatabase.postgresqlPhysical,
                  username: e.target.value.trim(),
                },
              });
              setIsConnectionTested(false);
            }}
            size="small"
            className="max-w-[200px] grow"
            placeholder="Enter PG username"
          />
        </div>

        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">Password</div>
          <Input.Password
            value={editingDatabase.postgresqlPhysical?.password}
            onChange={(e) => {
              if (!editingDatabase.postgresqlPhysical) return;

              setEditingDatabase({
                ...editingDatabase,
                postgresqlPhysical: {
                  ...editingDatabase.postgresqlPhysical,
                  password: e.target.value,
                },
              });
              setIsConnectionTested(false);
            }}
            size="small"
            className="max-w-[200px] grow"
            placeholder="Enter PG password"
            autoComplete="off"
            data-1p-ignore
            data-lpignore="true"
            data-form-type="other"
          />
        </div>

        <div className="mb-1 flex w-full items-center">
          <div className="min-w-[150px]">SSL mode</div>
          <Select
            value={editingDatabase.postgresqlPhysical?.sslMode ?? PostgresSslMode.Disable}
            onChange={(value: PostgresSslMode) => {
              if (!editingDatabase.postgresqlPhysical) return;

              setHasUserChosenSslMode(true);
              setEditingDatabase({
                ...editingDatabase,
                postgresqlPhysical: applySslMode(editingDatabase.postgresqlPhysical, value),
              });
              setIsConnectionTested(false);
            }}
            options={[
              { label: 'Disable', value: PostgresSslMode.Disable },
              { label: 'Require', value: PostgresSslMode.Require },
              { label: 'Verify CA', value: PostgresSslMode.VerifyCa },
              { label: 'Verify full', value: PostgresSslMode.VerifyFull },
            ]}
            size="small"
            className="max-w-[200px] grow"
          />
        </div>

        {renderSslCertSection()}

        {detectedVersion ? (
          <div className="mt-1 mb-1 flex w-full items-center">
            <div className="min-w-[150px]">PG version</div>
            <div>{postgresqlVersionLabels[detectedVersion] ?? detectedVersion}</div>
          </div>
        ) : (
          <div className="mt-1 mb-1 flex w-full items-center">
            <div className="min-w-[150px]">PG version</div>
            <div className="text-xs text-gray-500 dark:text-gray-400">
              Detected after test connection
            </div>
          </div>
        )}

        {renderFooter(
          <>
            {!isConnectionTested && (
              <Button
                type="primary"
                onClick={() => testConnection()}
                loading={isTestingConnection}
                disabled={!isAllFieldsFilled}
                className="mr-5"
              >
                Test connection
              </Button>
            )}

            {isConnectionTested && (
              <Button
                type="primary"
                onClick={() => saveDatabase()}
                loading={isSaving}
                disabled={!isAllFieldsFilled}
                className="mr-5"
              >
                {saveButtonText || 'Save'}
              </Button>
            )}
          </>,
        )}

        {isConnectionFailed && !IS_CLOUD && (
          <div className="mt-3 text-sm text-gray-500 dark:text-gray-400">
            If your database uses IP whitelist, make sure Databasus server IP is added to the
            allowed list.
          </div>
        )}
      </>
    );
  };

  return (
    <div>
      {renderForm()}

      <ClipboardPasteModalComponent
        open={isShowPasteModal}
        onSubmit={(text) => {
          setIsShowPasteModal(false);
          applyConnectionString(text);
        }}
        onCancel={() => setIsShowPasteModal(false)}
      />
    </div>
  );
};
