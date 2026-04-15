import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, TextInput, PasswordInput, Select, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsStorage() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [enabled, setEnabled] = useState(false);
  const [storageType, setStorageType] = useState("nfs");
  const [nfsMountPoint, setNfsMountPoint] = useState("");
  const [s3Bucket, setS3Bucket] = useState("");
  const [s3Region, setS3Region] = useState("");
  const [s3AccessKey, setS3AccessKey] = useState("");
  const [s3SecretKey, setS3SecretKey] = useState("");
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const st = (configData["storage.remote"] ?? {}) as any;
      setEnabled(st.enabled ?? false);
      setStorageType(st.type ?? "nfs");
      setNfsMountPoint(st.nfs?.mountPoint ?? "");
      setS3Bucket(st.s3?.bucket ?? "");
      setS3Region(st.s3?.region ?? "");
      setS3AccessKey(st.s3?.accessKey ?? "");
      setS3SecretKey(st.s3?.secretKey ?? "");
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/storage.remote", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: {
        enabled, type: storageType,
        nfs: { mountPoint: nfsMountPoint },
        s3: { bucket: s3Bucket, region: s3Region, accessKey: s3AccessKey, secretKey: s3SecretKey },
      }}),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Remote Storage</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable remote sync" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} mb="md" />
        <Select label="Storage Type" data={[
          { value: "nfs", label: "NFS Mount" },
          { value: "smb", label: "SMB/CIFS Share" },
          { value: "s3", label: "S3 / MinIO" },
        ]} value={storageType} onChange={v => setStorageType(v ?? "nfs")} disabled={!enabled} mb="md" />

        {storageType === "nfs" && (
          <TextInput label="NFS Mount Point" value={nfsMountPoint} onChange={e => setNfsMountPoint(e.currentTarget.value)}
            placeholder="/mnt/nas/novasky" disabled={!enabled} />
        )}
        {storageType === "s3" && (
          <Stack gap="sm">
            <TextInput label="Bucket" value={s3Bucket} onChange={e => setS3Bucket(e.currentTarget.value)} disabled={!enabled} />
            <TextInput label="Region" value={s3Region} onChange={e => setS3Region(e.currentTarget.value)} disabled={!enabled} />
            <TextInput label="Access Key" value={s3AccessKey} onChange={e => setS3AccessKey(e.currentTarget.value)} disabled={!enabled} />
            <PasswordInput label="Secret Key" value={s3SecretKey} onChange={e => setS3SecretKey(e.currentTarget.value)} disabled={!enabled} />
          </Stack>
        )}
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Storage Settings</Button>
    </Stack>
  );
}
