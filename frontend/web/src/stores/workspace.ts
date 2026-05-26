import { create } from "zustand";
import { workspaceServiceClient } from "@/grpcweb";
import { WorkspaceProfile, WorkspaceSetting } from "@/types/proto/api/v1/workspace_service";

interface WorkspaceState {
  profile: WorkspaceProfile;
  setting: WorkspaceSetting;

  fetchWorkspaceProfile: () => Promise<WorkspaceProfile>;
  fetchWorkspaceSetting: () => Promise<WorkspaceSetting>;
}

const useWorkspaceStore = create<WorkspaceState>()((set, get) => ({
  profile: WorkspaceProfile.fromPartial({}),
  setting: WorkspaceSetting.fromPartial({}),
  fetchWorkspaceProfile: async () => {
    const workspaceProfile = await workspaceServiceClient.getWorkspaceProfile({});
    set({ ...get(), profile: workspaceProfile });
    return workspaceProfile;
  },
  fetchWorkspaceSetting: async () => {
    const workspaceSetting = await workspaceServiceClient.getWorkspaceSetting({});
    set({ ...get(), setting: workspaceSetting });
    return workspaceSetting;
  },
}));

export default useWorkspaceStore;
