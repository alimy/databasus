import { ExclamationCircleOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { Button, Input, Select, Switch, Tooltip } from 'antd';
import { useEffect, useState } from 'react';

import { IS_CLOUD } from '../../../../constants';
import {
  type Storage,
  StorageType,
  getStorageLogoFromType,
  storageApi,
} from '../../../../entity/storages';
import { type UserProfile, UserRole } from '../../../../entity/users';
import { ToastHelper } from '../../../../shared/toast';
import { EditAzureBlobStorageComponent } from './storages/EditAzureBlobStorageComponent';
import { EditFTPStorageComponent } from './storages/EditFTPStorageComponent';
import { EditGoogleDriveStorageComponent } from './storages/EditGoogleDriveStorageComponent';
import { EditLocalStorageComponent } from './storages/EditLocalStorageComponent';
import { EditNASStorageComponent } from './storages/EditNASStorageComponent';
import { EditRcloneStorageComponent } from './storages/EditRcloneStorageComponent';
import { EditS3StorageComponent } from './storages/EditS3StorageComponent';
import { EditSFTPStorageComponent } from './storages/EditSFTPStorageComponent';

interface Props {
  workspaceId: string;

  isShowClose: boolean;
  onClose: () => void;

  isShowName: boolean;

  editingStorage?: Storage;
  onChanged: (storage: Storage) => void;

  user: UserProfile;
}

export function EditStorageComponent({
  workspaceId,
  isShowClose,
  onClose,
  isShowName,
  editingStorage,
  onChanged,
  user,
}: Props) {
  const [storage, setStorage] = useState<Storage | undefined>();
  const [isUnsaved, setIsUnsaved] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [isTestConnectionSuccess, setIsTestConnectionSuccess] = useState(false);
  const [connectionError, setConnectionError] = useState<string | undefined>();

  const save = async () => {
    if (!storage) return;

    setIsSaving(true);

    try {
      await storageApi.saveStorage(storage);
      onChanged(storage);
      setIsUnsaved(false);
    } catch (e) {
      alert((e as Error).message);
    }

    setIsSaving(false);
  };

  const testConnection = async () => {
    if (!storage) return;

    setIsTestingConnection(true);
    setConnectionError(undefined);

    try {
      await storageApi.testStorageConnectionDirect(storage);
      setIsTestConnectionSuccess(true);
      ToastHelper.showToast({
        title: 'Connection test successful!',
        description: 'Storage connection tested successfully',
      });
    } catch (e) {
      const errorMessage = (e as Error).message;
      setConnectionError(errorMessage);
      alert(errorMessage);
    }

    setIsTestingConnection(false);
  };

  const setStorageType = (type: StorageType) => {
    if (!storage) return;

    storage.localStorage = undefined;
    storage.s3Storage = undefined;
    storage.googleDriveStorage = undefined;
    storage.azureBlobStorage = undefined;
    storage.ftpStorage = undefined;
    storage.sftpStorage = undefined;
    storage.rcloneStorage = undefined;

    if (type === StorageType.LOCAL) {
      storage.localStorage = {};
    }

    if (type === StorageType.S3) {
      storage.s3Storage = {
        s3Bucket: '',
        s3Region: '',
        s3AccessKey: '',
        s3SecretKey: '',
        s3Endpoint: '',
      };
    }

    if (type === StorageType.GOOGLE_DRIVE) {
      storage.googleDriveStorage = {
        clientId: '',
        clientSecret: '',
      };
    }

    if (type === StorageType.NAS) {
      storage.nasStorage = {
        host: '',
        port: 445,
        share: '',
        username: '',
        password: '',
        useSsl: false,
        domain: '',
        path: '',
      };
    }

    if (type === StorageType.AZURE_BLOB) {
      storage.azureBlobStorage = {
        authMethod: 'ACCOUNT_KEY',
        connectionString: '',
        accountName: '',
        accountKey: '',
        containerName: '',
        endpoint: '',
        prefix: '',
      };
    }

    if (type === StorageType.FTP) {
      storage.ftpStorage = {
        host: '',
        port: 21,
        username: '',
        password: '',
        useSsl: false,
        path: '',
      };
    }

    if (type === StorageType.SFTP) {
      storage.sftpStorage = {
        host: '',
        port: 22,
        username: '',
        password: '',
        path: '',
      };
    }

    if (type === StorageType.RCLONE) {
      storage.rcloneStorage = {
        configContent: '',
        remotePath: '',
      };
    }

    setStorage(
      JSON.parse(
        JSON.stringify({
          ...storage,
          type: type,
        }),
      ),
    );
  };

  useEffect(() => {
    setIsUnsaved(true);

    setStorage(
      editingStorage
        ? JSON.parse(JSON.stringify(editingStorage))
        : {
            id: undefined as unknown as string,
            workspaceId,
            name: '',
            type: IS_CLOUD ? StorageType.S3 : StorageType.LOCAL,
            isSystem: false,
            localStorage: IS_CLOUD ? undefined : {},
            s3Storage: IS_CLOUD
              ? {
                  s3Bucket: '',
                  s3Region: '',
                  s3AccessKey: '',
                  s3SecretKey: '',
                  s3Endpoint: '',
                }
              : undefined,
          },
    );
  }, [editingStorage]);

  const isAllDataFilled = () => {
    if (!storage) {
      return false;
    }

    if (!storage.name) {
      return false;
    }

    if (storage.type === StorageType.LOCAL) {
      return true; // No additional settings required for local storage
    }

    if (storage.type === StorageType.S3) {
      if (storage.id) {
        return storage.s3Storage?.s3Bucket;
      }

      return (
        storage.s3Storage?.s3Bucket &&
        storage.s3Storage?.s3AccessKey &&
        storage.s3Storage?.s3SecretKey
      );
    }

    if (storage.type === StorageType.GOOGLE_DRIVE) {
      if (storage.id) {
        return storage.googleDriveStorage?.clientId;
      }

      return (
        storage.googleDriveStorage?.clientId &&
        storage.googleDriveStorage?.clientSecret &&
        storage.googleDriveStorage?.tokenJson
      );
    }

    if (storage.type === StorageType.NAS) {
      if (storage.id) {
        return (
          storage.nasStorage?.host &&
          storage.nasStorage?.port &&
          storage.nasStorage?.share &&
          storage.nasStorage?.username
        );
      }

      return (
        storage.nasStorage?.host &&
        storage.nasStorage?.port &&
        storage.nasStorage?.share &&
        storage.nasStorage?.username &&
        storage.nasStorage?.password
      );
    }

    if (storage.type === StorageType.AZURE_BLOB) {
      if (storage.id) {
        return storage.azureBlobStorage?.containerName;
      }

      const isContainerNameFilled = storage.azureBlobStorage?.containerName;

      if (storage.azureBlobStorage?.authMethod === 'CONNECTION_STRING') {
        return isContainerNameFilled && storage.azureBlobStorage?.connectionString;
      }

      if (storage.azureBlobStorage?.authMethod === 'ACCOUNT_KEY') {
        return (
          isContainerNameFilled &&
          storage.azureBlobStorage?.accountName &&
          storage.azureBlobStorage?.accountKey
        );
      }
    }

    if (storage.type === StorageType.FTP) {
      if (storage.id) {
        return storage.ftpStorage?.host && storage.ftpStorage?.port && storage.ftpStorage?.username;
      }

      return (
        storage.ftpStorage?.host &&
        storage.ftpStorage?.port &&
        storage.ftpStorage?.username &&
        storage.ftpStorage?.password
      );
    }

    if (storage.type === StorageType.SFTP) {
      if (storage.id) {
        return (
          storage.sftpStorage?.host && storage.sftpStorage?.port && storage.sftpStorage?.username
        );
      }

      return (
        storage.sftpStorage?.host &&
        storage.sftpStorage?.port &&
        storage.sftpStorage?.username &&
        (storage.sftpStorage?.password || storage.sftpStorage?.privateKey)
      );
    }

    if (storage.type === StorageType.RCLONE) {
      if (storage.id) {
        return true;
      }

      return storage.rcloneStorage?.configContent;
    }

    return false;
  };

  if (!storage) return <div />;

  const storageTypeOptions = [
    { label: 'Local storage', value: StorageType.LOCAL },
    { label: 'S3', value: StorageType.S3 },
    { label: 'Google Drive', value: StorageType.GOOGLE_DRIVE },
    { label: 'NAS', value: StorageType.NAS },
    { label: 'Azure Blob Storage', value: StorageType.AZURE_BLOB },
    { label: 'FTP', value: StorageType.FTP },
    { label: 'SFTP', value: StorageType.SFTP },
    { label: 'Rclone', value: StorageType.RCLONE },
  ].filter((option) => {
    if (IS_CLOUD && option.value === StorageType.LOCAL && user.role !== UserRole.ADMIN) {
      return false;
    }
    return true;
  });

  return (
    <div>
      {isShowName && (
        <div className="mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
          <div className="mb-1 min-w-[110px] sm:mb-0">Name</div>

          <Input
            value={storage?.name || ''}
            onChange={(e) => {
              setStorage({ ...storage, name: e.target.value });
              setIsUnsaved(true);
            }}
            size="small"
            className="w-full max-w-[250px]"
            placeholder="My Storage"
          />
        </div>
      )}

      <div className="mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
        <div className="mb-1 min-w-[110px] sm:mb-0">Type</div>

        <div className="flex items-center">
          <Select
            value={storage?.type}
            options={storageTypeOptions}
            onChange={(value) => {
              setStorageType(value);
              setIsUnsaved(true);
            }}
            size="small"
            className="w-[250px] max-w-[250px]"
          />

          <img src={getStorageLogoFromType(storage?.type)} className="ml-2 h-4 w-4" />
        </div>
      </div>

      {IS_CLOUD && user.role === UserRole.ADMIN && (
        <div className="mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
          <div className="mb-1 min-w-[110px] sm:mb-0">Is system?</div>

          <div className="flex items-center">
            <Switch
              checked={storage?.isSystem || false}
              disabled={!!storage?.id && storage?.isSystem}
              onChange={(checked) => {
                setStorage({ ...storage, isSystem: checked });
                setIsUnsaved(true);
              }}
              size="small"
            />

            <Tooltip
              className="cursor-pointer"
              title="System storage is accessible by all workspaces in this instance. Regular storage is only accessible by the current workspace."
            >
              <InfoCircleOutlined className="ml-2" style={{ color: 'gray' }} />
            </Tooltip>
          </div>
        </div>
      )}

      <div className="mt-5" />

      <div>
        {storage?.type === StorageType.S3 && (
          <EditS3StorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
              setConnectionError(undefined);
            }}
            connectionError={connectionError}
          />
        )}

        {storage?.type === StorageType.GOOGLE_DRIVE && (
          <EditGoogleDriveStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.NAS && (
          <EditNASStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.AZURE_BLOB && (
          <EditAzureBlobStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.FTP && (
          <EditFTPStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.SFTP && (
          <EditSFTPStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.RCLONE && (
          <EditRcloneStorageComponent
            storage={storage}
            setStorage={setStorage}
            setUnsaved={() => {
              setIsUnsaved(true);
              setIsTestConnectionSuccess(false);
            }}
          />
        )}

        {storage?.type === StorageType.LOCAL && <EditLocalStorageComponent />}
      </div>

      <div>
        {!IS_CLOUD && (
          <div className="mb-3 rounded bg-yellow-50 p-3 shadow dark:bg-yellow-900/30">
            <div className="mb-1 flex items-center gap-1.5 text-sm font-bold text-yellow-700 dark:text-yellow-400">
              <ExclamationCircleOutlined />
              Self-hosted notice
            </div>

            <div className="text-sm !text-yellow-600 dark:!text-yellow-500">
              Do not forget to backup the storage itself as it contains all your backups.
              <br /> Or you can use cloud{"'"}s build-in{' '}
              <u>unlimited storage with double reservation</u>. We care about security, maintainance
              and 24x7 uptime for you
            </div>

            <a
              href="https://databasus.com/cloud"
              target="_blank"
              rel="noreferrer"
              className="mt-2 block w-full rounded-md !bg-green-600 px-4 py-1.5 text-center text-sm font-medium !text-white transition-colors hover:!bg-green-700 dark:!bg-green-700 dark:hover:!bg-green-800"
            >
              Use cloud storage from $9
            </a>
          </div>
        )}
      </div>

      <div className="mt-3 flex">
        {isUnsaved && !isTestConnectionSuccess ? (
          <Button
            className="mr-1"
            disabled={isTestingConnection || !isAllDataFilled()}
            loading={isTestingConnection}
            type="primary"
            onClick={testConnection}
          >
            Test connection
          </Button>
        ) : (
          <div />
        )}

        {isUnsaved && isTestConnectionSuccess ? (
          <Button
            className="mr-1"
            disabled={isSaving || !isAllDataFilled()}
            loading={isSaving}
            type="primary"
            onClick={save}
          >
            Save
          </Button>
        ) : (
          <div />
        )}

        {isShowClose ? (
          <Button
            className="mr-1"
            disabled={isSaving}
            type="primary"
            danger
            ghost
            onClick={onClose}
          >
            Cancel
          </Button>
        ) : (
          <div />
        )}
      </div>
    </div>
  );
}
