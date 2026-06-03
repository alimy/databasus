import { InfoCircleOutlined } from '@ant-design/icons';
import { Button, Checkbox, InputNumber, Modal, Select, Spin, Switch, Tooltip } from 'antd';
import { type JSX, useEffect, useState } from 'react';

import { IS_CLOUD } from '../../../../constants';
import {
  type FullBackupsRetention,
  type PhysicalBackupConfig,
  PhysicalBackupNotificationType,
  PhysicalFullBackupsPolicy,
  PhysicalRetention,
  physicalBackupConfigApi,
} from '../../../../entity/backups/physical';
import { BackupEncryption } from '../../../../entity/backups/shared';
import { type Database, PhysicalDatabaseBackupType } from '../../../../entity/databases';
import { type Interval, IntervalType } from '../../../../entity/intervals';
import { type Storage, getStorageLogoFromType, storageApi } from '../../../../entity/storages';
import type { UserProfile } from '../../../../entity/users';
import { ConfirmationComponent } from '../../../../shared/ui';
import { EditStorageComponent } from '../../../storages/ui/edit/EditStorageComponent';
import { PhysicalIntervalEditor } from './PhysicalIntervalEditor';

interface Props {
  user: UserProfile;
  database: Database;

  isShowBackButton: boolean;
  onBack: () => void;

  isShowCancelButton?: boolean;
  onCancel: () => void;

  saveButtonText?: string;
  isSaveToApi: boolean;
  onSaved: (backupConfig: PhysicalBackupConfig) => void;
}

const BYTES_IN_MB = 1024 * 1024;

const isFullBackupsRetentionValid = (retention: FullBackupsRetention): boolean => {
  if (retention.policy === PhysicalFullBackupsPolicy.LAST_N) {
    return retention.count > 0;
  }

  return (
    retention.gfsHours > 0 ||
    retention.gfsDays > 0 ||
    retention.gfsWeeks > 0 ||
    retention.gfsMonths > 0 ||
    retention.gfsYears > 0
  );
};

const isIntervalValid = (interval?: Interval): boolean => {
  if (!interval?.type) return false;

  if (interval.type === IntervalType.WEEKLY) return Boolean(interval.weekday);
  if (interval.type === IntervalType.MONTHLY) return Boolean(interval.dayOfMonth);
  if (interval.type === IntervalType.CRON) return Boolean(interval.cronExpression);

  return true;
};

export const EditPhysicalBackupConfigComponent = ({
  user,
  database,

  isShowBackButton,
  onBack,

  isShowCancelButton,
  onCancel,
  saveButtonText,
  isSaveToApi,
  onSaved,
}: Props): JSX.Element => {
  const [backupConfig, setBackupConfig] = useState<PhysicalBackupConfig>();
  const [isUnsaved, setIsUnsaved] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  const [storages, setStorages] = useState<Storage[]>([]);
  const [isShowCreateStorage, setShowCreateStorage] = useState(false);
  const [storageSelectKey, setStorageSelectKey] = useState(0);

  const [isShowWarn, setIsShowWarn] = useState(false);

  // DB-level strategy lives on the database, not the config. It decides which
  // cadences and retention modes the config may use.
  const backupType = database.postgresqlPhysical?.backupType ?? PhysicalDatabaseBackupType.FULL;
  const isIncrementalAllowed = backupType !== PhysicalDatabaseBackupType.FULL;
  const isWalStream = backupType === PhysicalDatabaseBackupType.FULL_INCREMENTAL_WAL_STREAM;

  const updateBackupConfig = (patch: Partial<PhysicalBackupConfig>) => {
    setBackupConfig((prev) => (prev ? { ...prev, ...patch } : prev));
    setIsUnsaved(true);
  };

  const updateFullInterval = (patch: Partial<Interval>) => {
    setBackupConfig((prev) => {
      if (!prev) return prev;
      const merged = { ...(prev.fullBackupInterval ?? {}), ...patch } as Interval;
      return { ...prev, fullBackupInterval: merged };
    });
    setIsUnsaved(true);
  };

  const updateIncrementalInterval = (patch: Partial<Interval>) => {
    setBackupConfig((prev) => {
      if (!prev) return prev;
      const merged = { ...(prev.incrementalBackupInterval ?? {}), ...patch } as Interval;
      return { ...prev, incrementalBackupInterval: merged };
    });
    setIsUnsaved(true);
  };

  const updateFullBackupsRetention = (patch: Partial<FullBackupsRetention>) => {
    setBackupConfig((prev) =>
      prev ? { ...prev, fullBackupsRetention: { ...prev.fullBackupsRetention, ...patch } } : prev,
    );
    setIsUnsaved(true);
  };

  const toggleNotification = (type: PhysicalBackupNotificationType, isChecked: boolean) => {
    setBackupConfig((prev) => {
      if (!prev) return prev;
      const notifications = prev.sendNotificationsOn.filter((n) => n !== type);
      if (isChecked) notifications.push(type);
      return { ...prev, sendNotificationsOn: notifications };
    });
    setIsUnsaved(true);
  };

  const saveBackupConfig = async () => {
    if (!backupConfig) return;

    if (isSaveToApi) {
      setIsSaving(true);
      try {
        await physicalBackupConfigApi.savePhysicalBackupConfig(backupConfig);
        setIsUnsaved(false);
      } catch (e) {
        alert((e as Error).message);
      }
      setIsSaving(false);
    }

    onSaved(backupConfig);
  };

  const loadStorages = async () => {
    try {
      const loadedStorages = await storageApi.getStorages(database.workspaceId);
      setStorages(loadedStorages);

      if (IS_CLOUD) {
        const systemStorages = loadedStorages.filter((s) => s.isSystem);
        if (systemStorages.length > 0) {
          updateBackupConfig({ storage: systemStorages[0] });
        }
      }
    } catch (e) {
      alert((e as Error).message);
    }
  };

  const buildDefaultConfig = (): PhysicalBackupConfig => ({
    databaseId: database.id,
    isBackupsEnabled: true,
    fullBackupInterval: { type: IntervalType.DAILY, timeOfDay: '00:00' },
    incrementalBackupInterval: isIncrementalAllowed
      ? { type: IntervalType.HOURLY, timeOfDay: '00:00' }
      : undefined,
    retention:
      backupType === PhysicalDatabaseBackupType.FULL
        ? PhysicalRetention.FULL_BACKUPS
        : PhysicalRetention.CHAINS_AND_FULL_BACKUPS,
    chainsRetention: { count: 7 },
    fullBackupsRetention: {
      policy: PhysicalFullBackupsPolicy.GFS,
      count: 0,
      gfsHours: 0,
      gfsDays: 7,
      gfsWeeks: 4,
      gfsMonths: 12,
      gfsYears: 3,
    },
    walLagThresholdBytes: isWalStream ? 16 * BYTES_IN_MB : 0,
    storage: undefined,
    encryption: IS_CLOUD ? BackupEncryption.ENCRYPTED : BackupEncryption.NONE,
    sendNotificationsOn: [PhysicalBackupNotificationType.BACKUP_FAILED],
  });

  useEffect(() => {
    const run = async () => {
      setIsLoading(true);

      try {
        if (database.id) {
          const config = await physicalBackupConfigApi.getPhysicalBackupConfigByDbId(database.id);
          setBackupConfig(config);
          setIsUnsaved(false);
          setIsSaving(false);
        } else {
          setBackupConfig(buildDefaultConfig());
        }

        await loadStorages();
      } catch (e) {
        alert((e as Error).message);
      } finally {
        setIsLoading(false);
      }
    };

    run();
  }, [database]);

  if (isLoading) {
    return (
      <div className="mb-5 flex items-center">
        <Spin />
      </div>
    );
  }

  if (!backupConfig) return <div />;

  const fullBackupsRetention = backupConfig.fullBackupsRetention;

  // FULL databases are forced to FULL_BACKUPS retention; the others may pick
  // between CHAINS and CHAINS_AND_FULL_BACKUPS.
  const isShowChainsCount =
    backupType !== PhysicalDatabaseBackupType.FULL &&
    (backupConfig.retention === PhysicalRetention.CHAINS ||
      backupConfig.retention === PhysicalRetention.CHAINS_AND_FULL_BACKUPS);

  const isShowFullBackupsEditor =
    backupType === PhysicalDatabaseBackupType.FULL ||
    backupConfig.retention === PhysicalRetention.CHAINS_AND_FULL_BACKUPS;

  const isChainsCountValid = !isShowChainsCount || (backupConfig.chainsRetention?.count ?? 0) > 0;

  const isFullRetentionValid =
    !isShowFullBackupsEditor || isFullBackupsRetentionValid(fullBackupsRetention);

  const isWalThresholdValid = !isWalStream || backupConfig.walLagThresholdBytes > 0;

  const isAllFieldsFilled =
    !backupConfig.isBackupsEnabled ||
    (Boolean(backupConfig.storage?.id) &&
      Boolean(backupConfig.encryption) &&
      isIntervalValid(backupConfig.fullBackupInterval) &&
      (!isIncrementalAllowed || isIntervalValid(backupConfig.incrementalBackupInterval)) &&
      isChainsCountValid &&
      isFullRetentionValid &&
      isWalThresholdValid);

  return (
    <div>
      {database.id && (
        <div className="mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
          <div className="mb-1 min-w-[180px] sm:mb-0">Backups enabled</div>
          <Switch
            checked={backupConfig.isBackupsEnabled}
            onChange={(checked) => updateBackupConfig({ isBackupsEnabled: checked })}
            size="small"
          />
        </div>
      )}

      {backupConfig.isBackupsEnabled && (
        <>
          <PhysicalIntervalEditor
            label="Full backup cadence"
            interval={backupConfig.fullBackupInterval}
            onChange={updateFullInterval}
          />

          {isIncrementalAllowed && (
            <>
              <PhysicalIntervalEditor
                label="Incremental backup cadence"
                interval={backupConfig.incrementalBackupInterval}
                onChange={updateIncrementalInterval}
              />
              <div className="mt-1 mb-3 flex w-full flex-col items-start sm:flex-row sm:items-center">
                <div className="min-w-[180px]" />
                <div className="max-w-[320px] text-xs text-gray-500 dark:text-gray-400">
                  Incremental backups must run more frequently than full backups.
                </div>
              </div>
            </>
          )}

          {isWalStream && (
            <div className="mt-2 mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
              <div className="mb-1 min-w-[180px] sm:mb-0">WAL lag threshold (MB)</div>
              <div className="flex items-center">
                <InputNumber
                  min={1}
                  value={Math.round(backupConfig.walLagThresholdBytes / BYTES_IN_MB)}
                  onChange={(v) =>
                    updateBackupConfig({ walLagThresholdBytes: (v ?? 1) * BYTES_IN_MB })
                  }
                  size="small"
                  className="w-[100px]"
                />
                <Tooltip
                  className="cursor-pointer"
                  title="When the replication slot falls this far behind, the WAL stream is rebuilt. Must be greater than 0."
                >
                  <InfoCircleOutlined className="ml-2" style={{ color: 'gray' }} />
                </Tooltip>
              </div>
            </div>
          )}

          <div className="mb-3" />
        </>
      )}

      <div className="mt-5 mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
        <div className="mb-1 min-w-[180px] sm:mb-0">Storage</div>
        <div className="flex w-full items-center">
          <Select
            key={storageSelectKey}
            value={backupConfig.storage?.id}
            onChange={(storageId) => {
              if (storageId.includes('create-new-storage')) {
                setShowCreateStorage(true);
                return;
              }

              const selectedStorage = storages.find((s) => s.id === storageId);
              updateBackupConfig({ storage: selectedStorage });

              if (backupConfig.storage?.id) {
                setIsShowWarn(true);
              }
            }}
            size="small"
            className="mr-2 max-w-[200px] grow"
            options={[
              ...storages.map((s) => ({ label: s.name, value: s.id })),
              { label: 'Create new storage', value: 'create-new-storage' },
            ]}
            placeholder="Select storage"
          />

          {backupConfig.storage?.type && (
            <img
              src={getStorageLogoFromType(backupConfig.storage.type)}
              alt="storageIcon"
              className="ml-1 h-4 w-4"
            />
          )}
        </div>
      </div>

      {!IS_CLOUD && (
        <div className="mb-1 flex w-full flex-col items-start sm:flex-row sm:items-center">
          <div className="mb-1 min-w-[180px] sm:mb-0">Encryption</div>
          <div className="flex items-center">
            <Select
              value={backupConfig.encryption}
              onChange={(v) => updateBackupConfig({ encryption: v })}
              size="small"
              className="w-[200px]"
              options={[
                { label: 'None', value: BackupEncryption.NONE },
                { label: 'Encrypt backup files', value: BackupEncryption.ENCRYPTED },
              ]}
            />

            <Tooltip
              className="cursor-pointer"
              title="If backups are encrypted, the files in your storage cannot be used directly. You can restore them through Databasus or download them unencrypted."
            >
              <InfoCircleOutlined className="ml-2" style={{ color: 'gray' }} />
            </Tooltip>
          </div>
        </div>
      )}

      <div className="mt-5 mb-1 flex w-full flex-col items-start sm:flex-row sm:items-start">
        <div className="mt-1 mb-1 min-w-[180px] sm:mb-0">Retention</div>
        <div className="flex flex-col gap-2">
          {backupType === PhysicalDatabaseBackupType.FULL ? (
            <div className="max-w-[320px] text-xs text-gray-500 dark:text-gray-400">
              This database keeps full backups only, so retention applies to full backups.
            </div>
          ) : (
            <Select
              value={backupConfig.retention}
              onChange={(v) => updateBackupConfig({ retention: v })}
              size="small"
              className="w-[260px]"
              popupMatchSelectWidth={false}
              options={[
                { label: 'Keep last N chains', value: PhysicalRetention.CHAINS },
                {
                  label: 'Keep last N chains and full backups',
                  value: PhysicalRetention.CHAINS_AND_FULL_BACKUPS,
                },
              ]}
            />
          )}

          {isShowChainsCount && (
            <div className="flex items-center gap-2">
              <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">Chains</span>
              <InputNumber
                min={1}
                value={backupConfig.chainsRetention?.count}
                onChange={(v) => updateBackupConfig({ chainsRetention: { count: v ?? 1 } })}
                size="small"
                className="w-[80px]"
              />
              <Tooltip
                className="cursor-pointer"
                title="Number of most recent backup chains to keep. A chain is a full backup plus its incrementals."
              >
                <InfoCircleOutlined style={{ color: 'gray' }} />
              </Tooltip>
            </div>
          )}

          {isShowFullBackupsEditor && (
            <div className="mt-1 flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                  Full backups
                </span>
                <Select
                  value={fullBackupsRetention.policy}
                  onChange={(policy) => updateFullBackupsRetention({ policy })}
                  size="small"
                  className="w-[200px]"
                  popupMatchSelectWidth={false}
                  options={[
                    {
                      label: 'Count (last N full backups)',
                      value: PhysicalFullBackupsPolicy.LAST_N,
                    },
                    {
                      label: 'GFS (hourly, daily, weekly, monthly, yearly)',
                      value: PhysicalFullBackupsPolicy.GFS,
                    },
                  ]}
                />
              </div>

              {fullBackupsRetention.policy === PhysicalFullBackupsPolicy.LAST_N && (
                <div className="flex items-center gap-2">
                  <InputNumber
                    min={1}
                    value={fullBackupsRetention.count}
                    onChange={(v) => updateFullBackupsRetention({ count: v ?? 1 })}
                    size="small"
                    className="w-[80px]"
                  />
                  <span className="text-sm text-gray-600 dark:text-gray-400">
                    most recent full backups
                  </span>
                </div>
              )}

              {fullBackupsRetention.policy === PhysicalFullBackupsPolicy.GFS && (
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                      Hourly
                    </span>
                    <InputNumber
                      min={0}
                      value={fullBackupsRetention.gfsHours}
                      onChange={(v) => updateFullBackupsRetention({ gfsHours: v ?? 0 })}
                      size="small"
                      className="w-[80px]"
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                      Daily
                    </span>
                    <InputNumber
                      min={0}
                      value={fullBackupsRetention.gfsDays}
                      onChange={(v) => updateFullBackupsRetention({ gfsDays: v ?? 0 })}
                      size="small"
                      className="w-[80px]"
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                      Weekly
                    </span>
                    <InputNumber
                      min={0}
                      value={fullBackupsRetention.gfsWeeks}
                      onChange={(v) => updateFullBackupsRetention({ gfsWeeks: v ?? 0 })}
                      size="small"
                      className="w-[80px]"
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                      Monthly
                    </span>
                    <InputNumber
                      min={0}
                      value={fullBackupsRetention.gfsMonths}
                      onChange={(v) => updateFullBackupsRetention({ gfsMonths: v ?? 0 })}
                      size="small"
                      className="w-[80px]"
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="w-[110px] text-sm text-gray-600 dark:text-gray-400">
                      Yearly
                    </span>
                    <InputNumber
                      min={0}
                      value={fullBackupsRetention.gfsYears}
                      onChange={(v) => updateFullBackupsRetention({ gfsYears: v ?? 0 })}
                      size="small"
                      className="w-[80px]"
                    />
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {backupConfig.isBackupsEnabled && (
        <div className="mt-4 mb-1 flex w-full flex-col items-start sm:flex-row sm:items-start">
          <div className="mt-0 mb-1 min-w-[180px] sm:mt-1 sm:mb-0">Notifications</div>
          <div className="flex flex-col space-y-2">
            <Checkbox
              checked={backupConfig.sendNotificationsOn.includes(
                PhysicalBackupNotificationType.BACKUP_SUCCESS,
              )}
              onChange={(e) =>
                toggleNotification(PhysicalBackupNotificationType.BACKUP_SUCCESS, e.target.checked)
              }
            >
              Backup success
            </Checkbox>

            <Checkbox
              checked={backupConfig.sendNotificationsOn.includes(
                PhysicalBackupNotificationType.BACKUP_FAILED,
              )}
              onChange={(e) =>
                toggleNotification(PhysicalBackupNotificationType.BACKUP_FAILED, e.target.checked)
              }
            >
              Backup failed
            </Checkbox>

            {isIncrementalAllowed && (
              <Checkbox
                checked={backupConfig.sendNotificationsOn.includes(
                  PhysicalBackupNotificationType.CHAIN_BROKEN,
                )}
                onChange={(e) =>
                  toggleNotification(PhysicalBackupNotificationType.CHAIN_BROKEN, e.target.checked)
                }
              >
                Chain broken
              </Checkbox>
            )}

            {isWalStream && (
              <Checkbox
                checked={backupConfig.sendNotificationsOn.includes(
                  PhysicalBackupNotificationType.WAL_GAP,
                )}
                onChange={(e) =>
                  toggleNotification(PhysicalBackupNotificationType.WAL_GAP, e.target.checked)
                }
              >
                WAL gap
              </Checkbox>
            )}
          </div>
        </div>
      )}

      <div className="mt-5 flex">
        {isShowBackButton && (
          <Button className="mr-1" type="primary" ghost onClick={onBack}>
            Back
          </Button>
        )}

        {isShowCancelButton && (
          <Button danger ghost className="mr-1" onClick={onCancel}>
            Cancel
          </Button>
        )}

        <Button
          type="primary"
          className={`${isShowCancelButton ? 'ml-1' : 'ml-auto'} mr-5`}
          onClick={saveBackupConfig}
          loading={isSaving}
          disabled={!isUnsaved || !isAllFieldsFilled}
        >
          {saveButtonText || 'Save'}
        </Button>
      </div>

      {isShowCreateStorage && (
        <Modal
          title="Add storage"
          footer={<div />}
          open={isShowCreateStorage}
          onCancel={() => {
            setShowCreateStorage(false);
            setStorageSelectKey((prev) => prev + 1);
          }}
          maskClosable={false}
        >
          <div className="my-3 max-w-[275px] text-gray-500 dark:text-gray-400">
            Storage - is a place where backups will be stored (local disk, S3, Google Drive, etc.)
          </div>

          <EditStorageComponent
            user={user}
            workspaceId={database.workspaceId}
            isShowName
            isShowClose={false}
            onClose={() => setShowCreateStorage(false)}
            onChanged={() => {
              loadStorages();
              setShowCreateStorage(false);
            }}
          />
        </Modal>
      )}

      {isShowWarn && (
        <ConfirmationComponent
          onConfirm={() => setIsShowWarn(false)}
          onDecline={() => setIsShowWarn(false)}
          description="If you change the storage, all backups in this storage will be deleted."
          actionButtonColor="red"
          actionText="I understand"
          cancelText="Cancel"
          hideCancelButton
        />
      )}
    </div>
  );
};
