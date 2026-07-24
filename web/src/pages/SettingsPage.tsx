import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FolderSearch, Play, RotateCw, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { DirectoryBrowser } from '../components/DirectoryBrowser';
import { ProgressPanel } from '../components/ProgressPanel';
import { api } from '../lib/api';

export function SettingsPage() {
  const [path, setPath] = useState('');
  const [browserOpen, setBrowserOpen] = useState(false);
  const [databaseDir, setDatabaseDir] = useState('');
  const [databaseBrowserOpen, setDatabaseBrowserOpen] = useState(false);
  const [downloaderConcurrency, setDownloaderConcurrency] = useState(1);
  const queryClient = useQueryClient();

  const database = useQuery({ queryKey: ['database-config'], queryFn: api.databaseConfig });
  const directories = useQuery({ queryKey: ['directories'], queryFn: api.directories });
  const status = useQuery({ queryKey: ['scan-status'], queryFn: api.scanStatus, refetchInterval: 3000 });
  const updateAuth = useQuery({ queryKey: ['update-auth'], queryFn: api.updateAuth });

  useEffect(() => {
    const value = updateAuth.data?.auth.downloader_concurrency;
    if (value) setDownloaderConcurrency(value);
  }, [updateAuth.data?.auth.downloader_concurrency]);

  const saveDatabase = useMutation({
    mutationFn: (value: string) => api.saveDatabaseDirectory(value),
    onSuccess: (result) => {
      setDatabaseDir('');
      queryClient.setQueryData(['database-config'], result);
      queryClient.invalidateQueries({ queryKey: ['directories'] });
      queryClient.invalidateQueries({ queryKey: ['creators'] });
      queryClient.invalidateQueries({ queryKey: ['feed'] });
      queryClient.invalidateQueries({ queryKey: ['scan-status'] });
    },
  });

  const save = useMutation({
    mutationFn: (value: string) => api.saveDirectory(value),
    onSuccess: () => {
      setPath('');
      queryClient.invalidateQueries({ queryKey: ['directories'] });
    },
  });
  const remove = useMutation({
    mutationFn: api.deleteDirectory,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['directories'] });
      queryClient.invalidateQueries({ queryKey: ['creators'] });
      queryClient.invalidateQueries({ queryKey: ['feed'] });
    },
  });
  const scanOne = useMutation({ mutationFn: api.scanDirectory });
  const scanAll = useMutation({ mutationFn: api.scanAll });
  const saveDownloaderConcurrency = useMutation({
    mutationFn: (value: number) => api.saveUpdateAuth({ downloader_concurrency: value }),
    onSuccess: (result) => {
      setDownloaderConcurrency(result.auth.downloader_concurrency || 1);
      queryClient.invalidateQueries({ queryKey: ['update-auth'] });
    },
  });
  const savedDirectories = directories.data?.directories ?? [];

  return (
    <div className="page settings-page">
      <header className="page-header">
        <h1>目录配置</h1>
        <p>选择运行服务机器上的本地 Twitter 数据和数据库目录</p>
      </header>

      <section className="settings-layout">
        <div className="settings-main">
          <section className="settings-card">
            <h2>数据库位置</h2>
            <form
              className="path-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (databaseDir.trim()) saveDatabase.mutate(databaseDir.trim());
              }}
            >
              <input
                value={databaseDir}
                onChange={(event) => setDatabaseDir(event.target.value)}
                placeholder={database.data?.database.directory ?? '/path/to/localtwitter-data'}
              />
              <button type="button" onClick={() => setDatabaseBrowserOpen(true)} title="浏览数据库目录">
                <FolderSearch size={18} /> 浏览
              </button>
              <button type="submit">保存</button>
            </form>
            {saveDatabase.error && <p className="error-line">{saveDatabase.error.message}</p>}
            {database.data?.database && (
              <p className="config-line">
                当前数据库：{database.data.database.path}
                <span>{database.data.database.writable ? '可写' : '不可写'}</span>
              </p>
            )}
          </section>

          <section className="settings-card">
            <h2>Twitter 目录</h2>
            <form
              className="path-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (path.trim()) save.mutate(path.trim());
              }}
            >
              <input value={path} onChange={(event) => setPath(event.target.value)} placeholder="/path/to/twitter" />
              <button type="button" onClick={() => setBrowserOpen(true)} title="浏览目录">
                <FolderSearch size={18} /> 浏览
              </button>
              <button type="submit">保存</button>
            </form>
            {save.error && <p className="error-line">{save.error.message}</p>}

            <div className="directory-table">
              {savedDirectories.map((directory) => (
                <div className="directory-row" key={directory.id}>
                  <div>
                    <strong>{directory.name}</strong>
                    <span>{directory.path}</span>
                    <small>
                      {directory.creator_count} 博主 · {directory.work_count} 作品 · {directory.status}
                    </small>
                  </div>
                  <button onClick={() => scanOne.mutate(directory.id)} title="扫描此目录">
                    <Play size={17} /> 扫描
                  </button>
                  <button className="danger" onClick={() => remove.mutate(directory.id)} title="删除目录">
                    <Trash2 size={17} />
                  </button>
                </div>
              ))}
            </div>
            <button className="scan-all" onClick={() => scanAll.mutate()} disabled={!savedDirectories.length}>
              <RotateCw size={18} /> 扫描全部目录
            </button>
          </section>

          <section className="settings-card">
            <h2>更新下载器</h2>
            <form
              className="path-form"
              onSubmit={(event) => {
                event.preventDefault();
                saveDownloaderConcurrency.mutate(downloaderConcurrency);
              }}
            >
              <input
                aria-label="下载器并发数"
                type="number"
                min={1}
                max={8}
                value={downloaderConcurrency}
                onChange={(event) => setDownloaderConcurrency(Number(event.target.value) || 1)}
              />
              <button type="submit" disabled={saveDownloaderConcurrency.isPending}>
                保存
              </button>
            </form>
            <p className="config-line">
              同时运行的下载器数量：{updateAuth.data?.auth.downloader_concurrency ?? downloaderConcurrency}
              <span>范围 1-8，默认 1</span>
            </p>
            {saveDownloaderConcurrency.error && <p className="error-line">{saveDownloaderConcurrency.error.message}</p>}
          </section>
        </div>
        <ProgressPanel status={status.data?.status} />
      </section>

      <DirectoryBrowser
        open={browserOpen}
        onClose={() => setBrowserOpen(false)}
        onSelect={(selected) => {
          setPath(selected);
          setBrowserOpen(false);
        }}
      />
      <DirectoryBrowser
        open={databaseBrowserOpen}
        onClose={() => setDatabaseBrowserOpen(false)}
        onSelect={(selected) => {
          setDatabaseDir(selected);
          setDatabaseBrowserOpen(false);
        }}
      />
    </div>
  );
}
