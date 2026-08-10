import { NavActions } from './NavActions';

const Header = () => {
  return (
    <header className="flex justify-between  px-4 py-2">
      <div className="flex items-center">
        <div>ALemonTestOne</div>
      </div>
      <NavActions autoTooltipPlacement="bottom" />
    </header>
  );
};

export default Header;
