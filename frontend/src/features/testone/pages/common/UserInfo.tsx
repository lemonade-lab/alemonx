import { User } from '@testone/typing';
import Select from '@testone/ui/Select';

const UserInfo = ({
  user,
  users,
  onSelect
}: {
  user: User;
  users: User[];
  onUpdate?: (user: User) => void;
  onSelect: (user: User) => void;
}) => {
  return (
    <div className="flex items-center gap-2">
      <img
        src={user.UserAvatar}
        alt={user.UserName}
        className="w-8 h-8 rounded-full"
      />
      <div className="flex flex-col">
        <Select
          value={user.UserId}
          options={users.map(u => ({
            ...u,
            value: u.UserId,
            label: u.UserName
          }))}
          onSelect={value => {
            const user = users.find(u => u.UserId === value);
            if (!user) {
              return;
            }
            onSelect(user);
          }}
        />
        <span className="font-semibold">{user.UserId}</span>
      </div>
    </div>
  );
};

export default UserInfo;
