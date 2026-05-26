import { toast } from "sonner";
import WorkspaceGeneralSettingSection from "@/components/setting/WorkspaceGeneralSettingSection";
import WorkspaceMembersSection from "@/components/setting/WorkspaceMembersSection";
import WorkspaceSecuritySection from "@/components/setting/WorkspaceSecuritySection";
import { Separator } from "@/components/ui/separator";
import { useUserStore } from "@/stores";
import { Role } from "@/types/proto/api/v1/user_service";

const WorkspaceSetting = () => {
  const currentUser = useUserStore().getCurrentUser();
  const isAdmin = currentUser.role === Role.ADMIN;

  if (!isAdmin) {
    toast.error("Only workspace admins can access workspace settings.");
    return null;
  }

  return (
    <div className="mx-auto max-w-8xl w-full px-4 sm:px-6 md:px-12 py-6 flex flex-col justify-start items-start gap-y-12">
      <WorkspaceMembersSection />
      <Separator />
      <WorkspaceGeneralSettingSection />
      <Separator />
      <WorkspaceSecuritySection />
    </div>
  );
};

export default WorkspaceSetting;
