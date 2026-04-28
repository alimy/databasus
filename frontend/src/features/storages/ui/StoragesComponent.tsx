import { ExclamationCircleOutlined } from '@ant-design/icons';
import { Button, Modal, Spin } from 'antd';
import { useEffect, useState } from 'react';

import { IS_CLOUD } from '../../../constants';
import { storageApi } from '../../../entity/storages';
import type { Storage } from '../../../entity/storages';
import type { UserProfile } from '../../../entity/users';
import type { WorkspaceResponse } from '../../../entity/workspaces';
import { useIsMobile } from '../../../shared/hooks';
import { StorageCardComponent } from './StorageCardComponent';
import { StorageComponent } from './StorageComponent';
import { EditStorageComponent } from './edit/EditStorageComponent';

interface Props {
  user: UserProfile;
  contentHeight: number;
  workspace: WorkspaceResponse;
  isCanManageStorages: boolean;
}

const SELECTED_STORAGE_STORAGE_KEY = 'selectedStorageId';

export const StoragesComponent = ({
  user,
  contentHeight,
  workspace,
  isCanManageStorages,
}: Props) => {
  const isMobile = useIsMobile();
  const [isLoading, setIsLoading] = useState(true);
  const [storages, setStorages] = useState<Storage[]>([]);

  const [isShowAddStorage, setIsShowAddStorage] = useState(false);
  const [selectedStorageId, setSelectedStorageId] = useState<string | undefined>(undefined);

  const updateSelectedStorageId = (storageId: string | undefined) => {
    setSelectedStorageId(storageId);
    if (storageId) {
      localStorage.setItem(`${SELECTED_STORAGE_STORAGE_KEY}_${workspace.id}`, storageId);
    } else {
      localStorage.removeItem(`${SELECTED_STORAGE_STORAGE_KEY}_${workspace.id}`);
    }
  };

  const loadStorages = (isSilent = false, selectStorageId?: string) => {
    if (!isSilent) {
      setIsLoading(true);
    }

    storageApi
      .getStorages(workspace.id)
      .then((storages: Storage[]) => {
        setStorages(storages);
        if (selectStorageId) {
          updateSelectedStorageId(selectStorageId);
        } else if (!selectedStorageId && !isSilent && !isMobile) {
          // On desktop, auto-select a storage; on mobile, keep it unselected to show the list first
          const savedStorageId = localStorage.getItem(
            `${SELECTED_STORAGE_STORAGE_KEY}_${workspace.id}`,
          );
          const storageToSelect =
            savedStorageId && storages.some((s) => s.id === savedStorageId)
              ? savedStorageId
              : storages[0]?.id;
          updateSelectedStorageId(storageToSelect);
        }
      })
      .catch((e: Error) => alert(e.message))
      .finally(() => setIsLoading(false));
  };

  useEffect(() => {
    loadStorages();
  }, []);

  if (isLoading) {
    return (
      <div className="mx-3 my-3 flex w-[250px] justify-center">
        <Spin />
      </div>
    );
  }

  const addStorageButton = (
    <Button type="primary" className="mb-2 w-full" onClick={() => setIsShowAddStorage(true)}>
      Add storage
    </Button>
  );

  // On mobile, show either the list or the storage details
  const showStorageList = !isMobile || !selectedStorageId;
  const showStorageDetails = selectedStorageId && (!isMobile || selectedStorageId);

  return (
    <>
      <div className="flex grow">
        {showStorageList && (
          <div
            className="w-full overflow-y-auto md:mx-3 md:w-[250px] md:min-w-[250px] md:pr-2"
            style={{ height: contentHeight }}
          >
            {storages.length >= 5 && isCanManageStorages && addStorageButton}

            {!IS_CLOUD && (
              <div className="mb-3 rounded bg-yellow-50 p-3 shadow dark:bg-yellow-900/30">
                <div className="mb-1 flex items-center gap-1.5 text-sm font-bold text-yellow-700 dark:text-yellow-400">
                  <ExclamationCircleOutlined />
                  Self-hosted notice
                </div>

                <div className="text-sm !text-yellow-600 dark:!text-yellow-500">
                  Do not forget to backup the storage itself as it contains all your backups.
                  <br /> Or you can use cloud{"'"}s build-in{' '}
                  <u>unlimited storage with double reservation</u>. We care about security,
                  maintainance and 24x7 uptime
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

            {storages.map((storage) => (
              <StorageCardComponent
                key={storage.id}
                storage={storage}
                selectedStorageId={selectedStorageId}
                setSelectedStorageId={updateSelectedStorageId}
              />
            ))}

            {storages.length < 5 && isCanManageStorages && addStorageButton}

            <div className="mx-3 text-center text-xs text-gray-500 dark:text-gray-400">
              Storage - is a place where backups will be stored (local disk, S3, etc.)
            </div>
          </div>
        )}

        {showStorageDetails && (
          <div className="flex w-full flex-col md:flex-1">
            {isMobile && (
              <div className="mb-2">
                <Button
                  type="default"
                  onClick={() => updateSelectedStorageId(undefined)}
                  className="w-full"
                >
                  ← Back to storages
                </Button>
              </div>
            )}

            <StorageComponent
              storageId={selectedStorageId}
              onStorageChanged={() => {
                loadStorages();
              }}
              onStorageDeleted={() => {
                const remainingStorages = storages.filter(
                  (storage) => storage.id !== selectedStorageId,
                );
                updateSelectedStorageId(remainingStorages[0]?.id);
                loadStorages();
              }}
              onStorageTransferred={() => {
                const remainingStorages = storages.filter(
                  (storage) => storage.id !== selectedStorageId,
                );
                updateSelectedStorageId(remainingStorages[0]?.id);
                loadStorages();
              }}
              isCanManageStorages={isCanManageStorages}
              user={user}
            />
          </div>
        )}
      </div>

      {isShowAddStorage && (
        <Modal
          title="Add storage"
          footer={<div />}
          open={isShowAddStorage}
          onCancel={() => setIsShowAddStorage(false)}
          maskClosable={false}
        >
          <div className="my-3 max-w-[250px] text-gray-500 dark:text-gray-400">
            Storage - is a place where backups will be stored (local disk, S3, etc.)
          </div>

          <EditStorageComponent
            workspaceId={workspace.id}
            isShowName
            isShowClose={false}
            onClose={() => setIsShowAddStorage(false)}
            onChanged={(storage) => {
              loadStorages(false, storage.id);
              setIsShowAddStorage(false);
            }}
            user={user}
          />
        </Modal>
      )}
    </>
  );
};
