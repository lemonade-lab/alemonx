import { Channel } from '@testone/typing';
import { memo } from 'react';

const ChannelItem = ({
  channel,
  onSelect
}: {
  channel: Channel;
  onSelect: (channel: Channel) => void;
}) => {
  return (
    <div
      className="flex items-center p-2 hover:bg-[var(--list-hoverBackground)] cursor-pointer animate__animated animate__fadeInUp hover-lift"
      onClick={() => onSelect(channel)}
    >
      <img
        src={channel.ChannelAvatar}
        alt={channel.ChannelName}
        className="w-8 h-8 rounded-full mr-2"
      />
      <span className="text-sm">{channel.ChannelName}</span>
    </div>
  );
};

export default memo(ChannelItem);
